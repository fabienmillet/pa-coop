// pa-coop : auto-resynchronisation (Windows, The_Jailhouse_1.02 r11056, Steam et Epic).
//
// Même principe que la version Linux : au lieu d'afficher « sync error » et de déconnecter le joueur,
// le client renvoie à l'hôte sa demande de connexion ; l'hôte lui renvoie la prison complète.
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <time.h>

void pacoop_log(const char *fmt, ...);

// Adresses de l'exécutable 64 bits r11056 (base fixe 0x140000000, pas d'ASLR).
#define ADDR_NOTIFY_SYNC_ERROR 0x140427f50ULL // PhotonNetworkingManager::NotifySynchronisationError
#define ADDR_RPCOPTIONS_CTOR 0x140165830ULL   // RPCOptions::RPCOptions(int)
#define ADDR_RPC_STRING_CALL 0x140424150ULL   // RPC<std::string>::Call(RPCOptions const&, std::string)
#define ADDR_STRING_COPY 0x1400d4510ULL       // std::string::string(std::string const&)
#define OFF_AUTHORISE_RPC 0xb70
#define OFF_PASSWORD 0x3e0

static const unsigned char EXPECTED[16] = {0x48, 0x8b, 0xc4, 0x55, 0x41, 0x56, 0x41, 0x57,
                                           0x48, 0x8d, 0x68, 0xa1, 0x48, 0x81, 0xec, 0x90};

typedef void *(*string_copy_t)(void *dst, const void *src);
typedef void *(*rpcopt_ctor_t)(void *self, int target);
typedef void (*rpc_string_call_t)(void *rpc, const void *opts, void *arg);

static time_t g_last_resync;
static int g_resync_count;

static void start_watchdog(void *mgr);
static void send_request(void *mgr);

static void request_resync(void *mgr, const char *why) {
	time_t now = time(NULL);
	if (now - g_last_resync < 15) {
		pacoop_log("resynchronisation déjà demandée il y a %ld s (%s)", (long)(now - g_last_resync), why);
		return;
	}
	g_last_resync = now;
	g_resync_count++;
	send_request(mgr);
	pacoop_log("désynchronisation (%s) : prison redemandée à l'hôte (n°%d)", why, g_resync_count);
	start_watchdog(mgr);
}

static void send_request(void *mgr) {
	// std::string MSVC (32 octets) construite par le jeu ; l'appelé la détruit (convention MSVC).
	__attribute__((aligned(16))) unsigned char pw[64];
	__attribute__((aligned(16))) unsigned char opts[64];
	memset(pw, 0, sizeof pw);
	memset(opts, 0, sizeof opts);
	((string_copy_t)ADDR_STRING_COPY)(pw, (char *)mgr + OFF_PASSWORD);
	((rpcopt_ctor_t)ADDR_RPCOPTIONS_CTOR)(opts, 1);
	((rpc_string_call_t)ADDR_RPC_STRING_CALL)((char *)mgr + OFF_AUTHORISE_RPC, opts, pw);
}

// Surveille debug.txt : si le renvoi de la prison ne progresse plus pendant 25 s, on le redemande (3 essais).
static volatile LONG g_wd_running;

static int scan_log(const char *p, long *off) {
	FILE *f = fopen(p, "rb");
	if (!f) return 0;
	fseek(f, *off, SEEK_SET);
	char line[512];
	int r = 0;
	while (fgets(line, sizeof line, f)) {
		if (strstr(line, "Finished receiving save game")) r = 2;
		else if (r < 2 && (strstr(line, "SaveGameSender: Received chunk") || strstr(line, "SaveGameSender: Expecting"))) r = 1;
	}
	*off = ftell(f);
	fclose(f);
	return r;
}

static DWORD WINAPI watchdog(LPVOID mgr) {
	char path[MAX_PATH + 64], base[MAX_PATH];
	if (!GetEnvironmentVariableA("LOCALAPPDATA", base, sizeof base)) base[0] = 0;
	snprintf(path, sizeof path, "%s\\Introversion\\Prison Architect\\debug.txt", base);
	long off = 0;
	FILE *f = fopen(path, "rb");
	if (f) {
		fseek(f, 0, SEEK_END);
		off = ftell(f);
		fclose(f);
	}
	DWORD start = GetTickCount(), last = start;
	int tries = 1;
	for (;;) {
		Sleep(2000);
		int r = scan_log(path, &off);
		if (r == 2) {
			pacoop_log("prison rechargée en %lu s", (unsigned long)((GetTickCount() - start) / 1000));
			break;
		}
		if (r == 1) last = GetTickCount();
		if (GetTickCount() - last >= 25000) {
			if (tries >= 3) {
				pacoop_log("la prison n'arrive pas après %d demandes : abandon", tries);
				break;
			}
			tries++;
			send_request(mgr);
			last = GetTickCount();
			pacoop_log("renvoi bloqué : prison redemandée (essai %d)", tries);
		}
	}
	InterlockedExchange(&g_wd_running, 0);
	return 0;
}

static void start_watchdog(void *mgr) {
	if (InterlockedExchange(&g_wd_running, 1)) return;
	CreateThread(NULL, 0, watchdog, mgr, 0, NULL);
}

// std::string MSVC : données en ligne si capacité < 16, sinon pointeur ; taille à +0x10, capacité à +0x18.
static void msvc_string(const void *s, char *out, size_t n) {
	const unsigned char *b = s;
	size_t len = *(const size_t *)(b + 0x10), cap = *(const size_t *)(b + 0x18);
	const char *d = cap >= 16 ? *(const char *const *)b : (const char *)b;
	snprintf(out, n, "%.*s", (int)(len < n - 1 ? len : n - 1), d);
}

#define ADDR_G_APP 0x140cd7cc0ULL         // App* g_app
#define VTABLE_PHOTON_MANAGER 0x140a2be40ULL // vtable de PhotonNetworkingManager (RTTI)

static int readable(const void *p, size_t n) {
	MEMORY_BASIC_INFORMATION mi;
	if (!VirtualQuery(p, &mi, sizeof mi) || mi.State != MEM_COMMIT || (mi.Protect & (PAGE_NOACCESS | PAGE_GUARD))) return 0;
	return (const char *)p + n <= (const char *)mi.BaseAddress + mi.RegionSize;
}

// Retrouve le PhotonNetworkingManager parmi les membres de l'App (pointeur dont la vtable correspond).
static void *find_manager(void) {
	char *app = *(char **)(uintptr_t)ADDR_G_APP;
	if (!app) return NULL;
	for (size_t o = 0; o < 0x4000; o += 8) {
		if (!readable(app + o, 8)) break;
		void *p = *(void **)(app + o);
		if (p && ((uintptr_t)p & 7) == 0 && readable(p, 8) && *(uintptr_t *)p == VTABLE_PHOTON_MANAGER) {
			pacoop_log("test : manager trouvé à App+0x%zx", o);
			return p;
		}
	}
	return NULL;
}

static DWORD WINAPI test_thread(LPVOID arg) {
	char path[MAX_PATH + 32];
	snprintf(path, sizeof path, "%s\\pacoop-resync-test", (const char *)arg);
	for (;;) {
		Sleep(1000);
		if (GetFileAttributesA(path) == INVALID_FILE_ATTRIBUTES) continue;
		DeleteFileA(path);
		void *mgr = find_manager();
		if (!mgr) {
			pacoop_log("test : pas de partie en ligne");
			continue;
		}
		g_last_resync = 0;
		request_resync(mgr, "test manuel");
	}
	return 0;
}

void pacoop_start_resync_test(const char *dir) {
	CreateThread(NULL, 0, test_thread, (LPVOID)dir, 0, NULL);
	pacoop_log("déclencheur de test actif (fichier pacoop-resync-test)");
}

static void hook_notify_sync_error(void *mgr, const void *msg) {
	char why[128] = "";
	if (msg) msvc_string(msg, why, sizeof why);
	request_resync(mgr, why);
}

void pacoop_install_resync(void) {
	if (GetEnvironmentVariableA("PACOOP_NO_RESYNC", NULL, 0)) return;
	unsigned char *target = (unsigned char *)(uintptr_t)ADDR_NOTIFY_SYNC_ERROR;
	MEMORY_BASIC_INFORMATION mi;
	if (!VirtualQuery(target, &mi, sizeof mi) || mi.State != MEM_COMMIT || memcmp(target, EXPECTED, sizeof EXPECTED) != 0) {
		pacoop_log("version du jeu inattendue : auto-resynchronisation désactivée");
		return;
	}
	unsigned char jmp[12] = {0x48, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xe0}; // movabs rax, hook ; jmp rax
	uint64_t a = (uint64_t)(uintptr_t)hook_notify_sync_error;
	memcpy(jmp + 2, &a, 8);
	DWORD old;
	VirtualProtect(target, sizeof jmp, PAGE_EXECUTE_READWRITE, &old);
	memcpy(target, jmp, sizeof jmp);
	VirtualProtect(target, sizeof jmp, old, &old);
	FlushInstructionCache(GetCurrentProcess(), target, sizeof jmp);
	pacoop_log("auto-resynchronisation active");
}
