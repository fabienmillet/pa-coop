// pa-coop : mod Linux de Prison Architect (bibliothèque chargée par LD_PRELOAD).
//
// Redirige le multijoueur (Photon) vers le serveur indiqué dans pacoop.txt (dossier du jeu)
// ou dans la variable PACOOP_SERVER, et lance ./pacoop-server avec server=local.
#define _GNU_SOURCE
#include <arpa/inet.h>
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <netdb.h>
#include <netinet/in.h>
#include <pthread.h>
#include <signal.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <unistd.h>
#include "../common/pacoop_cfg.h"

// Compatibilité avec les vieux runtimes Steam (glibc 2.28) : versions anciennes de ces symboles.
__asm__(".symver dlsym,dlsym@GLIBC_2.2.5");
__asm__(".symver pthread_once,pthread_once@GLIBC_2.2.5");

void pacoop_install_resync(void);
void pacoop_start_resync_test(void);

static struct pacoop_cfg g_cfg;
static pthread_once_t g_once = PTHREAD_ONCE_INIT;
static int (*real_getaddrinfo)(const char *, const char *, const struct addrinfo *, struct addrinfo **);
static int (*real_connect)(int, const struct sockaddr *, socklen_t);

static void spawn(char *const argv[], const char *logname) {
	if (access("./pacoop-server", X_OK) != 0) {
		fprintf(stderr, "[pa-coop] ./pacoop-server est absent\n");
		return;
	}
	pid_t pid = fork();
	if (pid == 0) {
		prctl(PR_SET_PDEATHSIG, SIGTERM);
		unsetenv("LD_PRELOAD");
		int fd = open(logname, O_WRONLY | O_CREAT | O_TRUNC, 0644);
		if (fd >= 0) {
			dup2(fd, 1);
			dup2(fd, 2);
		}
		execv("./pacoop-server", argv);
		_exit(127);
	}
	fprintf(stderr, "[pa-coop] %s lancé (pid %d)\n", argv[1] && strcmp(argv[1], "-ns-only") == 0 ? "sélecteur de serveurs" : "serveur", (int)pid);
}

static void start_selector(void) {
	char port[16];
	snprintf(port, sizeof port, "%d", PACOOP_SELECTOR_PORT);
	char *argv[] = {"pacoop-server", "-ns-only", "-ns-port", port, "-regions", g_cfg.servers, NULL};
	spawn(argv, "pacoop-selecteur.log");
}

static void start_local_server(void) {
	if (access("./pacoop-server", X_OK) != 0) {
		fprintf(stderr, "[pa-coop] server=local mais ./pacoop-server est absent\n");
		return;
	}
	pid_t parent = getpid();
	pid_t pid = fork();
	if (pid == 0) {
		prctl(PR_SET_PDEATHSIG, SIGTERM);
		unsetenv("LD_PRELOAD");
		int fd = open("pacoop-server.log", O_WRONLY | O_CREAT | O_TRUNC, 0644);
		if (fd >= 0) {
			dup2(fd, 1);
			dup2(fd, 2);
		}
		char ppid[16], ns[16], ms[16], gs[16];
		int off = g_cfg.port_offset;
		snprintf(ppid, sizeof ppid, "%d", (int)parent);
		snprintf(ns, sizeof ns, "%d", 4533 + off);
		snprintf(ms, sizeof ms, "%d", 4530 + off);
		snprintf(gs, sizeof gs, "%d", 4531 + off);
		if (g_cfg.upnp)
			execl("./pacoop-server", "pacoop-server", "-parent", ppid, "-ns-port", ns, "-master-port", ms, "-game-port", gs, "-upnp", (char *)NULL);
		else
			execl("./pacoop-server", "pacoop-server", "-parent", ppid, "-ns-port", ns, "-master-port", ms, "-game-port", gs, (char *)NULL);
		_exit(127);
	}
	fprintf(stderr, "[pa-coop] serveur local lancé (pid %d)\n", (int)pid);
}

static void init(void) {
	real_getaddrinfo = dlsym(RTLD_NEXT, "getaddrinfo");
	real_connect = dlsym(RTLD_NEXT, "connect");
	pacoop_read_cfg(&g_cfg, "pacoop.txt");
	const char *env = getenv("PACOOP_SERVER");
	if (env && *env) pacoop_set_server(&g_cfg, env);
	if (!g_cfg.host[0]) {
		fprintf(stderr, "[pa-coop] pas de serveur : jeu en ligne officiel\n");
		return;
	}
	if (g_cfg.nlist)
		fprintf(stderr, "[pa-coop] %d serveurs dans le menu Region : %s\n", g_cfg.nlist, g_cfg.servers);
	else
		fprintf(stderr, "[pa-coop] serveur : %s%s, décalage de ports %+d\n", g_cfg.local ? "local, " : "", g_cfg.host, g_cfg.port_offset);
}

// Au chargement du jeu (et seulement du jeu) : lit la config, lance le serveur local si demandé.
__attribute__((constructor)) static void pacoop_ctor(void) {
	char exe[512];
	ssize_t n = readlink("/proc/self/exe", exe, sizeof exe - 1);
	if (n <= 0) return;
	exe[n] = 0;
	if (!strstr(exe, "PrisonArchitect")) return;
	pthread_once(&g_once, init);
	if (g_cfg.nlist) start_selector();
	if (g_cfg.local || g_cfg.list_has_local) start_local_server();
	if (g_cfg.host[0]) pacoop_install_resync();
	if (g_cfg.host[0] && g_cfg.test) pacoop_start_resync_test();
}

int getaddrinfo(const char *node, const char *service, const struct addrinfo *hints, struct addrinfo **res) {
	pthread_once(&g_once, init);
	if (g_cfg.host[0] && pacoop_is_photon_host(node)) {
		const char *t = pacoop_target(&g_cfg, node);
		fprintf(stderr, "[pa-coop] redirection %s -> %s\n", node, t);
		node = t;
	} else {
		pacoop_note_host(&g_cfg, node);
	}
	return real_getaddrinfo(node, service, hints, res);
}

int connect(int fd, const struct sockaddr *a, socklen_t len) {
	pthread_once(&g_once, init);
	if (a && g_cfg.host[0]) {
		if (a->sa_family == AF_INET && len >= sizeof(struct sockaddr_in)) {
			struct sockaddr_in in = *(const struct sockaddr_in *)a;
			int lo = ((const unsigned char *)&in.sin_addr)[0] == 127;
			char ip[INET_ADDRSTRLEN];
			if (inet_ntop(AF_INET, &in.sin_addr, ip, sizeof ip)) pacoop_note_connect(&g_cfg, ip, ntohs(in.sin_port));
			int np = pacoop_remap_port(&g_cfg, ntohs(in.sin_port), lo);
			if (np) {
				in.sin_port = htons(np);
				return real_connect(fd, (struct sockaddr *)&in, sizeof in);
			}
		} else if (a->sa_family == AF_INET6 && len >= sizeof(struct sockaddr_in6)) {
			struct sockaddr_in6 in = *(const struct sockaddr_in6 *)a;
			int np = pacoop_remap_port(&g_cfg, ntohs(in.sin6_port), 0);
			if (np) {
				in.sin6_port = htons(np);
				return real_connect(fd, (struct sockaddr *)&in, sizeof in);
			}
		}
	}
	return real_connect(fd, a, len);
}
