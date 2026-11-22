// pa-coop : lecture de pacoop.txt, commune aux mods Windows et Linux.
//
// Format (une option par ligne, # = commentaire) :
//   server=adresse[:port]   serveur à utiliser (IP ou nom ; port du serveur de noms, 4533 par défaut)
//   server=local[:port]     lance le serveur pa-coop fourni avec le mod (pour héberger)
//   upnp=0|1                avec server=local : ouvrir les ports sur la box par UPnP (1 par défaut)
// Une ligne sans « = » est prise comme valeur de server.
#pragma once
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#ifdef _WIN32
#define strcasecmp _stricmp
#else
#include <strings.h>
#endif

#define PACOOP_NS_PORT 4533

struct pacoop_cfg {
	char host[256]; // hôte vers lequel rediriger ("" = pas de redirection)
	int port_offset; // port du serveur - 4533
	int local;      // 1 = lancer le serveur local
	int upnp;
	int test;       // 1 = déclencheur de test de la resynchronisation (fichier pacoop-resync-test)
	// servers=Nom=hôte[:port],… : liste proposée dans le menu Region du jeu
	char servers[1024];
	int nlist;
	char list_host[8][256];
	int list_master_port[8];
	int list_has_local;
	char current[256]; // serveur choisi dans le menu (traduction de pacoop.server)
};

#define PACOOP_SELECTOR_PORT 14599

static void pacoop_trim(char *s);
static int pacoop_atoi(const char *s);

static void pacoop_parse_list(struct pacoop_cfg *c) {
	char buf[1024];
	snprintf(buf, sizeof buf, "%s", c->servers);
	c->nlist = 0;
	for (char *it = strtok(buf, ","); it && c->nlist < 8; it = strtok(NULL, ",")) {
		char *eq = strchr(it, '=');
		if (!eq) continue;
		char *h = eq + 1;
		pacoop_trim(h);
		char host[256];
		snprintf(host, sizeof host, "%s", h);
		int nsport = PACOOP_NS_PORT;
		char *colon = strrchr(host, ':');
		if (colon && strchr(host, ':') == colon) {
			*colon = 0;
			int p = pacoop_atoi(colon + 1);
			if (p > 3 && p < 65536) nsport = p;
		}
		c->list_master_port[c->nlist] = nsport; // serveur pa-coop à port unique
		if (strcasecmp(host, "local") == 0) {
			snprintf(host, sizeof host, "127.0.0.1");
			c->list_has_local = 1;
		}
		snprintf(c->list_host[c->nlist++], sizeof c->list_host[0], "%s", host);
	}
	if (c->nlist) {
		snprintf(c->host, sizeof c->host, "127.0.0.1"); // le sélecteur local répond au jeu
		snprintf(c->current, sizeof c->current, "%s", c->list_host[0]);
		c->port_offset = 0;
	}
}

// Avec une liste : le jeu se connecte au maître du serveur choisi dans le menu Region ; on retient son adresse
// pour y envoyer ensuite la connexion au serveur de jeu (nom symbolique pacoop.server).
static void pacoop_note_connect(struct pacoop_cfg *c, const char *ip, int port) {
	if (!c->nlist) return;
	for (int i = 0; i < c->nlist; i++)
		if (c->list_master_port[i] == port) {
			snprintf(c->current, sizeof c->current, "%s", ip);
			return;
		}
}

// Avec une liste : retient le serveur que le jeu contacte (choisi dans le menu Region).
static void pacoop_note_host(struct pacoop_cfg *c, const char *node) {
	if (!node || !c->nlist) return;
	for (int i = 0; i < c->nlist; i++)
		if (strcasecmp(node, c->list_host[i]) == 0) snprintf(c->current, sizeof c->current, "%s", node);
}

// Entier décimal simple (évite atoi/strtol, versionnés trop récemment dans glibc).
static int pacoop_atoi(const char *s) {
	int v = 0;
	while (*s == ' ') s++;
	while (*s >= '0' && *s <= '9' && v < 1000000) v = v * 10 + (*s++ - '0');
	return v;
}

static void pacoop_trim(char *s) {
	char *e = s + strlen(s);
	while (e > s && (e[-1] == '\n' || e[-1] == '\r' || e[-1] == ' ' || e[-1] == '\t')) *--e = 0;
	char *b = s;
	while (*b == ' ' || *b == '\t') b++;
	memmove(s, b, strlen(b) + 1);
}

// Analyse "adresse[:port]" (IPv6 entre crochets accepté).
static void pacoop_set_server(struct pacoop_cfg *c, const char *v) {
	char buf[256];
	snprintf(buf, sizeof buf, "%s", v);
	int port = PACOOP_NS_PORT;
	char *h = buf, *colon = NULL;
	if (buf[0] == '[') {
		char *e = strchr(buf, ']');
		if (e) {
			*e = 0;
			h = buf + 1;
			if (e[1] == ':') colon = e + 1;
		}
	} else {
		colon = strrchr(buf, ':');
		if (colon && strchr(buf, ':') != colon) colon = NULL; // IPv6 nue : pas de port
	}
	if (colon) {
		*colon = 0;
		int p = pacoop_atoi(colon + 1);
		if (p > 0 && p < 65536) port = p;
	}
	c->port_offset = port - PACOOP_NS_PORT;
	if (strcasecmp(h, "local") == 0) {
		c->local = 1;
		snprintf(c->host, sizeof c->host, "127.0.0.1");
	} else {
		c->local = 0;
		snprintf(c->host, sizeof c->host, "%s", h);
	}
}

static int pacoop_read_cfg(struct pacoop_cfg *c, const char *path) {
	memset(c, 0, sizeof *c);
	c->upnp = 1;
	FILE *f = fopen(path, "r");
	if (!f) return 0;
	char line[512];
	while (fgets(line, sizeof line, f)) {
		pacoop_trim(line);
		if (!line[0] || line[0] == '#') continue;
		char *v = strchr(line, '=');
		if (!v) {
			if (!c->host[0]) pacoop_set_server(c, line);
			continue;
		}
		*v++ = 0;
		pacoop_trim(line);
		pacoop_trim(v);
		if (strcasecmp(line, "server") == 0) pacoop_set_server(c, v);
		else if (strcasecmp(line, "upnp") == 0) c->upnp = pacoop_atoi(v) != 0;
		else if (strcasecmp(line, "test") == 0) c->test = pacoop_atoi(v) != 0;
		else if (strcasecmp(line, "servers") == 0) snprintf(c->servers, sizeof c->servers, "%s", v);
	}
	fclose(f);
	if (c->servers[0]) pacoop_parse_list(c);
	return 1;
}

// Noms à rediriger : serveurs Photon et le nom symbolique renvoyé par pacoop-server.
static int pacoop_is_photon_host(const char *n) {
	if (!n) return 0;
	if (strcasecmp(n, "pacoop.server") == 0) return 1;
	size_t l = strlen(n);
	const char *suf[] = {".exitgames.com", ".photonengine.io", ".photonengine.com"};
	for (int i = 0; i < 3; i++) {
		size_t sl = strlen(suf[i]);
		if (l > sl && strcasecmp(n + l - sl, suf[i]) == 0) return 1;
	}
	return 0;
}


// Hôte à utiliser pour un nom à rediriger.
static const char *pacoop_target(struct pacoop_cfg *c, const char *node) {
	if (c->nlist && strcasecmp(node, "pacoop.server") == 0) return c->current;
	return c->host;
}

// Nouveau port de destination (0 = inchangé).
static int pacoop_remap_port(struct pacoop_cfg *c, int p, int is_loopback) {
	if (c->nlist) return (p == 4533 && is_loopback) ? PACOOP_SELECTOR_PORT : 0;
	// Le jeu ne choisit que le port du serveur de noms (4533) ; le serveur donne lui-même les autres adresses.
	if (c->port_offset && p == PACOOP_NS_PORT) return p + c->port_offset;
	return 0;
}
