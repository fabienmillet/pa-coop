// pa-coop : dbghelp.dll de remplacement pour Prison Architect (Windows).
//
// Posée dans le dossier du jeu, elle est chargée au démarrage à la place de celle du système.
// Elle transmet les appels dbghelp à la vraie DLL, redirige le multijoueur (Photon) vers le
// serveur indiqué dans pacoop.txt et, avec server=local, lance pacoop-server.exe (fourni à côté).
#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <stdarg.h>
#include "../common/pacoop_cfg.h"

// Pas d'import de ws2_32 depuis la DLL : conversions d'ordre d'octets locales.
#define p_ntohs(x) __builtin_bswap16(x)
#define p_htons(x) __builtin_bswap16(x)

static struct pacoop_cfg g_cfg;
static char g_dir[MAX_PATH];
static FILE *g_log;

void pacoop_install_resync(void);
void pacoop_start_resync_test(const char *dir);

void pacoop_log(const char *fmt, ...) {
	if (!g_log) return;
	va_list ap;
	va_start(ap, fmt);
	vfprintf(g_log, fmt, ap);
	va_end(ap);
	fputc('\n', g_log);
	fflush(g_log);
}

static void logf_(const char *fmt, ...) {
	if (!g_log) return;
	va_list ap;
	va_start(ap, fmt);
	vfprintf(g_log, fmt, ap);
	va_end(ap);
	fputc('\n', g_log);
	fflush(g_log);
}

// ---------- vraie dbghelp ----------

#define DBGHELP_FUNCS(X) \
	X(SymCleanup) X(SymGetOptions) X(SymLoadModule64) X(SymFromAddrW) X(SymGetSearchPath) \
	X(SymSetSearchPath) X(SymGetLineFromAddrW64) X(SymSetOptions) X(MiniDumpWriteDump) X(SymInitialize)

#define DECL(n) void *real_##n;
DBGHELP_FUNCS(DECL)

// Chaque export saute vers la fonction réelle (mêmes registres, même pile).
#define TRAMP(n) __asm__(".globl " #n "\n" #n ":\n\tjmp *real_" #n "(%rip)\n");
DBGHELP_FUNCS(TRAMP)

static void load_real_dbghelp(void) {
	char path[MAX_PATH];
	UINT n = GetSystemDirectoryA(path, MAX_PATH);
	if (n == 0 || n > MAX_PATH - 20) return;
	strcat(path, "\\dbghelp.dll");
	HMODULE h = LoadLibraryA(path);
	if (!h) {
		logf_("vraie dbghelp introuvable (%s)", path);
		return;
	}
#define RES(n) real_##n = (void *)GetProcAddress(h, #n);
	DBGHELP_FUNCS(RES)
}

// ---------- redirection ----------

typedef int(WSAAPI *getaddrinfo_t)(PCSTR, PCSTR, const ADDRINFOA *, PADDRINFOA *);
typedef int(WSAAPI *connect_t)(SOCKET, const struct sockaddr *, int);
static getaddrinfo_t real_getaddrinfo;
static connect_t real_connect;

static int WSAAPI hook_getaddrinfo(PCSTR node, PCSTR service, const ADDRINFOA *hints, PADDRINFOA *res) {
	if (pacoop_is_photon_host(node)) {
		const char *t = pacoop_target(&g_cfg, node);
		logf_("redirection %s -> %s", node, t);
		node = t;
	} else {
		pacoop_note_host(&g_cfg, node);
	}
	return real_getaddrinfo(node, service, hints, res);
}

// Décale les ports Photon (4530/4531/4533) quand le serveur utilise d'autres ports.
static int WSAAPI hook_connect(SOCKET s, const struct sockaddr *a, int len) {
	if (a && a->sa_family == AF_INET && len >= (int)sizeof(struct sockaddr_in)) {
		struct sockaddr_in in = *(const struct sockaddr_in *)a;
		const unsigned char *b = (const unsigned char *)&in.sin_addr;
		int lo = b[0] == 127;
		char ip[20];
		snprintf(ip, sizeof ip, "%u.%u.%u.%u", b[0], b[1], b[2], b[3]);
		pacoop_note_connect(&g_cfg, ip, p_ntohs(in.sin_port));
		int np = pacoop_remap_port(&g_cfg, p_ntohs(in.sin_port), lo);
		if (np) {
			in.sin_port = p_htons((u_short)np);
			return real_connect(s, (struct sockaddr *)&in, sizeof in);
		}
	} else if (a && a->sa_family == AF_INET6 && len >= (int)sizeof(struct sockaddr_in6)) {
		struct sockaddr_in6 in = *(const struct sockaddr_in6 *)a;
		int np = pacoop_remap_port(&g_cfg, p_ntohs(in.sin6_port), 0);
		if (np) {
			in.sin6_port = p_htons((u_short)np);
			return real_connect(s, (struct sockaddr *)&in, sizeof in);
		}
	}
	return real_connect(s, a, len);
}

// Remplace getaddrinfo (par nom) et connect (ordinal 4) dans les imports ws2_32 du jeu.
static void patch_iat(void) {
	BYTE *base = (BYTE *)GetModuleHandleA(NULL);
	IMAGE_NT_HEADERS *nt = (IMAGE_NT_HEADERS *)(base + ((IMAGE_DOS_HEADER *)base)->e_lfanew);
	IMAGE_DATA_DIRECTORY dir = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT];
	if (!dir.VirtualAddress) return;
	for (IMAGE_IMPORT_DESCRIPTOR *d = (void *)(base + dir.VirtualAddress); d->Name; d++) {
		if (_stricmp((char *)(base + d->Name), "ws2_32.dll") != 0) continue;
		IMAGE_THUNK_DATA *names = (void *)(base + (d->OriginalFirstThunk ? d->OriginalFirstThunk : d->FirstThunk));
		IMAGE_THUNK_DATA *iat = (void *)(base + d->FirstThunk);
		for (; names->u1.AddressOfData; names++, iat++) {
			void **slot = NULL, *hook = NULL;
			if (IMAGE_SNAP_BY_ORDINAL(names->u1.Ordinal)) {
				if (IMAGE_ORDINAL(names->u1.Ordinal) == 4) { // connect
					real_connect = (connect_t)iat->u1.Function;
					slot = (void **)&iat->u1.Function;
					hook = (void *)hook_connect;
				}
			} else {
				IMAGE_IMPORT_BY_NAME *ibn = (void *)(base + names->u1.AddressOfData);
				if (strcmp((char *)ibn->Name, "getaddrinfo") == 0) {
					real_getaddrinfo = (getaddrinfo_t)iat->u1.Function;
					slot = (void **)&iat->u1.Function;
					hook = (void *)hook_getaddrinfo;
				} else if (strcmp((char *)ibn->Name, "connect") == 0) {
					real_connect = (connect_t)iat->u1.Function;
					slot = (void **)&iat->u1.Function;
					hook = (void *)hook_connect;
				}
			}
			if (slot) {
				DWORD old;
				VirtualProtect(slot, sizeof(void *), PAGE_READWRITE, &old);
				*slot = hook;
				VirtualProtect(slot, sizeof(void *), old, &old);
			}
		}
	}
	logf_("interceptions : getaddrinfo %s, connect %s", real_getaddrinfo ? "oui" : "NON", real_connect ? "oui" : "NON");
}

// ---------- serveur local ----------

static void spawn_server(const char *args, const char *logname) {
	char exe[MAX_PATH + 32], cmd[2048], logp[MAX_PATH + 32];
	snprintf(exe, sizeof exe, "%s\\pacoop-server.exe", g_dir);
	if (GetFileAttributesA(exe) == INVALID_FILE_ATTRIBUTES) {
		logf_("pacoop-server.exe est absent du dossier du jeu");
		return;
	}
	snprintf(cmd, sizeof cmd, "\"%s\" -parent %lu %s", exe, (unsigned long)GetCurrentProcessId(), args);
	snprintf(logp, sizeof logp, "%s\\%s", g_dir, logname);
	SECURITY_ATTRIBUTES sa = {sizeof sa, NULL, TRUE};
	HANDLE lf = CreateFileA(logp, GENERIC_WRITE, FILE_SHARE_READ, &sa, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
	STARTUPINFOA si = {sizeof si};
	si.dwFlags = STARTF_USESTDHANDLES;
	si.hStdOutput = si.hStdError = lf;
	PROCESS_INFORMATION pi;
	if (CreateProcessA(NULL, cmd, NULL, NULL, TRUE, CREATE_NO_WINDOW, NULL, g_dir, &si, &pi)) {
		logf_("lancé (pid %lu) : %s", (unsigned long)pi.dwProcessId, cmd);
		CloseHandle(pi.hThread);
		CloseHandle(pi.hProcess);
	} else {
		logf_("échec du lancement (erreur %lu) : %s", (unsigned long)GetLastError(), cmd);
	}
	if (lf != INVALID_HANDLE_VALUE) CloseHandle(lf);
}

static void start_selector(void) {
	char args[1400];
	snprintf(args, sizeof args, "-ns-only -ns-port %d -regions \"%s\"", PACOOP_SELECTOR_PORT, g_cfg.servers);
	spawn_server(args, "pacoop-selecteur.log");
}

static void start_local_server(void) {
	char exe[MAX_PATH + 32], cmd[1024], logp[MAX_PATH + 32];
	snprintf(exe, sizeof exe, "%s\\pacoop-server.exe", g_dir);
	if (GetFileAttributesA(exe) == INVALID_FILE_ATTRIBUTES) {
		logf_("server=local mais pacoop-server.exe est absent du dossier du jeu");
		return;
	}
	int off = g_cfg.port_offset;
	snprintf(cmd, sizeof cmd, "\"%s\" -parent %lu -ns-port %d -master-port %d -game-port %d%s", exe,
	         (unsigned long)GetCurrentProcessId(), 4533 + off, 4530 + off, 4531 + off, g_cfg.upnp ? " -upnp" : "");
	snprintf(logp, sizeof logp, "%s\\pacoop-server.log", g_dir);
	SECURITY_ATTRIBUTES sa = {sizeof sa, NULL, TRUE};
	HANDLE lf = CreateFileA(logp, GENERIC_WRITE, FILE_SHARE_READ, &sa, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
	STARTUPINFOA si = {sizeof si};
	si.dwFlags = STARTF_USESTDHANDLES;
	si.hStdOutput = si.hStdError = lf;
	si.hStdInput = NULL;
	PROCESS_INFORMATION pi;
	if (CreateProcessA(NULL, cmd, NULL, NULL, TRUE, CREATE_NO_WINDOW, NULL, g_dir, &si, &pi)) {
		logf_("serveur local lancé (pid %lu) : %s", (unsigned long)pi.dwProcessId, cmd);
		CloseHandle(pi.hThread);
		CloseHandle(pi.hProcess);
	} else {
		logf_("échec du lancement du serveur local (erreur %lu)", (unsigned long)GetLastError());
	}
	if (lf != INVALID_HANDLE_VALUE) CloseHandle(lf);
}

BOOL WINAPI DllMain(HINSTANCE inst, DWORD reason, LPVOID res) {
	(void)res;
	if (reason != DLL_PROCESS_ATTACH) return TRUE;
	DisableThreadLibraryCalls(inst);
	GetModuleFileNameA(NULL, g_dir, MAX_PATH);
	char *s = strrchr(g_dir, '\\');
	if (s) *s = 0;
	char p[MAX_PATH + 32];
	snprintf(p, sizeof p, "%s\\pacoop.log", g_dir);
	g_log = fopen(p, "w");
	logf_("pa-coop chargé");
	load_real_dbghelp();
	snprintf(p, sizeof p, "%s\\pacoop.txt", g_dir);
	if (!pacoop_read_cfg(&g_cfg, p) || !g_cfg.host[0]) {
		logf_("pas de serveur dans pacoop.txt : jeu en ligne officiel");
		return TRUE;
	}
	if (g_cfg.nlist) logf_("liste de %d serveurs pour le menu Region : %s", g_cfg.nlist, g_cfg.servers);
	else logf_("serveur : %s%s, décalage de ports %+d", g_cfg.local ? "local, " : "", g_cfg.host, g_cfg.port_offset);
	patch_iat();
	if (g_cfg.nlist) start_selector();
	if (g_cfg.local || g_cfg.list_has_local) start_local_server();
	pacoop_install_resync();
	if (g_cfg.test) pacoop_start_resync_test(g_dir);
	return TRUE;
}
