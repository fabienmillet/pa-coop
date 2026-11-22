// pa-coop : auto-resynchronisation (Linux, The_Jailhouse_1.02 r11056).
//
// Quand le client détecte une désynchronisation (« Object List Sync Error »), le jeu appelle
// PhotonNetworkingManager::NotifySynchronisationError, qui affiche une erreur et déconnecte le joueur.
// On remplace cette fonction : le client renvoie à l'hôte sa demande de connexion, l'hôte lui renvoie
// la prison complète et le client la recharge, sans quitter la partie.
#define _GNU_SOURCE
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <time.h>
#include <unistd.h>
#include <pthread.h>
__asm__(".symver pthread_create,pthread_create@GLIBC_2.2.5");
__asm__(".symver pthread_detach,pthread_detach@GLIBC_2.2.5");
__asm__(".symver pthread_mutex_lock,pthread_mutex_lock@GLIBC_2.2.5");
__asm__(".symver pthread_mutex_unlock,pthread_mutex_unlock@GLIBC_2.2.5");

// Adresses de la version Linux r11056 (exécutable non relocalisable).
#define ADDR_NOTIFY_SYNC_ERROR 0x99f190UL // PhotonNetworkingManager::NotifySynchronisationError(std::string const&)
#define ADDR_RPCOPTIONS_CTOR 0xe07320UL   // RPCOptions::RPCOptions(int)
#define ADDR_RPC_STRING_CALL 0x9b5520UL   // RPC<std::string>::Call(RPCOptions const&, std::string)
#define OFF_AUTHORISE_RPC 0xc48           // membre RPC<std::string> « autoriser la connexion »
#define OFF_PASSWORD 0x4a0                // std::string mot de passe de la partie

static const unsigned char EXPECTED[16] = {0x55, 0x48, 0x8d, 0x97, 0xc0, 0x04, 0x00, 0x00,
                                           0x48, 0x89, 0xe5, 0x41, 0x56, 0x41, 0x54, 0x53};

// std::string (ABI C++11 de libstdc++)
struct cxx_string {
	char *p;
	size_t len;
	union {
		char buf[16];
		size_t cap;
	};
};

typedef void (*rpcopt_ctor_t)(void *self, int target);
typedef void (*rpc_string_call_t)(void *rpc, const void *opts, struct cxx_string *arg);

static time_t g_last_resync;
static int g_resync_count;

static void start_watchdog(void *mgr);

// Demande à l'hôte de renvoyer la prison (même message qu'au moment de rejoindre la partie).
static void send_request(void *mgr);

void pacoop_request_resync(void *mgr, const char *why) {
	time_t now = time(NULL);
	if (now - g_last_resync < 15) {
		fprintf(stderr, "[pa-coop] resynchronisation déjà demandée il y a %ld s (%s)\n", (long)(now - g_last_resync), why);
		return;
	}
	g_last_resync = now;
	g_resync_count++;
	send_request(mgr);
	fprintf(stderr, "[pa-coop] désynchronisation (%s) : prison redemandée à l'hôte (n°%d)\n", why, g_resync_count);
	start_watchdog(mgr);
}

static void send_request(void *mgr) {
	const struct cxx_string *pw = (const struct cxx_string *)((char *)mgr + OFF_PASSWORD);
	struct cxx_string copy;
	memset(&copy, 0, sizeof copy);
	if (pw->len < sizeof copy.buf) {
		copy.p = copy.buf;
		memcpy(copy.buf, pw->p, pw->len);
	} else {
		copy.p = malloc(pw->len + 1);
		memcpy(copy.p, pw->p, pw->len);
		copy.p[pw->len] = 0;
		copy.cap = pw->len;
	}
	copy.len = pw->len;
	unsigned char opts[64] __attribute__((aligned(16)));
	memset(opts, 0, sizeof opts);
	((rpcopt_ctor_t)ADDR_RPCOPTIONS_CTOR)(opts, 1);
	((rpc_string_call_t)ADDR_RPC_STRING_CALL)((char *)mgr + OFF_AUTHORISE_RPC, opts, &copy);
	if (copy.p != copy.buf) free(copy.p);
}

// Surveille le journal du jeu : si le renvoi de la prison ne progresse plus pendant 25 s, on le redemande
// (3 essais au plus).
static pthread_mutex_t g_wd_lock = PTHREAD_MUTEX_INITIALIZER;
static int g_wd_running;

static long file_size(const char *p) {
	FILE *f = fopen(p, "rb");
	if (!f) return -1;
	fseek(f, 0, SEEK_END);
	long n = ftell(f);
	fclose(f);
	return n;
}

// Lit le journal depuis off ; renvoie 2 si le transfert est fini, 1 s'il a progressé, 0 sinon.
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

static void *watchdog(void *mgr) {
	char path[1024];
	snprintf(path, sizeof path, "%s/.Prison Architect/debug.txt", getenv("HOME") ? getenv("HOME") : ".");
	long off = file_size(path);
	if (off < 0) off = 0;
	time_t start = time(NULL), last = start;
	int tries = 1;
	for (;;) {
		sleep(2);
		int r = scan_log(path, &off);
		if (r == 2) {
			fprintf(stderr, "[pa-coop] prison rechargée en %ld s\n", (long)(time(NULL) - start));
			break;
		}
		if (r == 1) last = time(NULL);
		if (time(NULL) - last >= 25) {
			if (tries >= 3) {
				fprintf(stderr, "[pa-coop] la prison n'arrive pas après %d demandes : abandon\n", tries);
				break;
			}
			tries++;
			send_request(mgr);
			last = time(NULL);
			fprintf(stderr, "[pa-coop] renvoi bloqué : prison redemandée (essai %d)\n", tries);
		}
	}
	pthread_mutex_lock(&g_wd_lock);
	g_wd_running = 0;
	pthread_mutex_unlock(&g_wd_lock);
	return NULL;
}

static void start_watchdog(void *mgr) {
	pthread_mutex_lock(&g_wd_lock);
	int already = g_wd_running;
	g_wd_running = 1;
	pthread_mutex_unlock(&g_wd_lock);
	if (already) return;
	pthread_t t;
	pthread_create(&t, NULL, watchdog, mgr);
	pthread_detach(t);
}

#define ADDR_G_APP 0x14c9fc0UL // App* g_app
#define OFF_APP_NETWORK 0xff8  // App::m_network (NetworkingManager*)

// Test : un fichier « pacoop-resync-test » dans le dossier du jeu déclenche une resynchronisation.
static void *test_thread(void *arg) {
	(void)arg;
	for (;;) {
		sleep(1);
		if (access("pacoop-resync-test", F_OK) != 0) continue;
		unlink("pacoop-resync-test");
		char *app = *(char **)ADDR_G_APP;
		void *mgr = app ? *(void **)(app + OFF_APP_NETWORK) : NULL;
		if (!mgr) {
			fprintf(stderr, "[pa-coop] test : pas de partie en ligne\n");
			continue;
		}
		g_last_resync = 0;
		pacoop_request_resync(mgr, "test manuel");
	}
	return NULL;
}

void pacoop_start_resync_test(void) {
	pthread_t t;
	pthread_create(&t, NULL, test_thread, NULL);
	pthread_detach(t);
	fprintf(stderr, "[pa-coop] déclencheur de test actif (fichier pacoop-resync-test)\n");
}

// Remplace PhotonNetworkingManager::NotifySynchronisationError.
static void hook_notify_sync_error(void *mgr, const struct cxx_string *msg) {
	char why[128];
	snprintf(why, sizeof why, "%.*s", msg ? (int)(msg->len < 100 ? msg->len : 100) : 0, msg ? msg->p : "");
	pacoop_request_resync(mgr, why);
}

// Installe le détournement si l'exécutable est bien la version attendue.
void pacoop_install_resync(void) {
	if (getenv("PACOOP_NO_RESYNC")) return;
	unsigned char *target = (unsigned char *)ADDR_NOTIFY_SYNC_ERROR;
	if (memcmp(target, EXPECTED, sizeof EXPECTED) != 0) {
		fprintf(stderr, "[pa-coop] version du jeu inattendue : auto-resynchronisation désactivée\n");
		return;
	}
	long pg = sysconf(_SC_PAGESIZE);
	uintptr_t start = (uintptr_t)target & ~(uintptr_t)(pg - 1);
	if (mprotect((void *)start, 2 * pg, PROT_READ | PROT_WRITE | PROT_EXEC) != 0) {
		perror("[pa-coop] mprotect");
		return;
	}
	// movabs rax, hook ; jmp rax
	unsigned char jmp[12] = {0x48, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xe0};
	uint64_t a = (uint64_t)(uintptr_t)hook_notify_sync_error;
	memcpy(jmp + 2, &a, 8);
	memcpy(target, jmp, sizeof jmp);
	mprotect((void *)start, 2 * pg, PROT_READ | PROT_EXEC);
	fprintf(stderr, "[pa-coop] auto-resynchronisation active\n");
}
