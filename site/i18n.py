#!/usr/bin/env python3
"""Builds the French pages (fr/index.html, fr/legal.html) from the English ones.

Each English string is replaced by its French translation; the script fails if a string is missing,
so a change to the English page can't silently leave untranslated text. Run after editing index.html or legal.html.
"""
import os
import re
import sys

D = os.path.dirname(os.path.abspath(__file__))
SITE = "https://pa-coop.zipname.fr"

INDEX = [
    # --- head
    ('<html lang="en">', '<html lang="fr">'),
    ('<title>pa-coop · Prison Architect co-op on your own server</title>', '<title>pa-coop · le co-op de Prison Architect sur votre propre serveur</title>'),
    ("pa-coop runs Prison Architect's online co-op on your own server: more than 4 players, Steam and Epic players in the same game, host from the game or run a dedicated server. Free and open source.",
     "pa-coop fait tourner le co-op en ligne de Prison Architect sur votre propre serveur : plus de 4 joueurs, Steam et Epic dans la même partie, hébergement depuis le jeu ou serveur dédié. Gratuit et open source."),
    ('<link rel="canonical" href="https://pa-coop.zipname.fr/">', '<link rel="canonical" href="https://pa-coop.zipname.fr/fr/">'),
    ('<meta property="og:url" content="https://pa-coop.zipname.fr/">', '<meta property="og:url" content="https://pa-coop.zipname.fr/fr/">'),
    ('content="pa-coop · Prison Architect co-op on your own server"', 'content="pa-coop · le co-op de Prison Architect sur votre propre serveur"'),
    ('content="More than 4 players, Steam and Epic in the same game, host from the game or run a dedicated server."',
     'content="Plus de 4 joueurs, Steam et Epic dans la même partie, hébergement depuis le jeu ou serveur dédié."'),
    ('<meta property="og:locale" content="en_US">\n<meta property="og:locale:alternate" content="fr_FR">',
     '<meta property="og:locale" content="fr_FR">\n<meta property="og:locale:alternate" content="en_US">'),
    ('content="pa-coop: self-hosted co-op for Prison Architect, over a screenshot of a prison"',
     'content="pa-coop : le co-op auto-hébergé pour Prison Architect, sur une capture d\'une prison"'),
    ('"inLanguage": "en"', '"inLanguage": "fr"'),
    ('"Self-hosted co-op server and client mod for Prison Architect: more than 4 players, Steam and Epic players in the same game, host from the game or run a dedicated server."',
     '"Serveur co-op auto-hébergé et mod client pour Prison Architect : plus de 4 joueurs, Steam et Epic dans la même partie, hébergement depuis le jeu ou serveur dédié."'),
    ('"url": "https://pa-coop.zipname.fr/",\n      "applicationCategory"', '"url": "https://pa-coop.zipname.fr/fr/",\n      "applicationCategory"'),
    ('"@id": "https://pa-coop.zipname.fr/#faq"', '"@id": "https://pa-coop.zipname.fr/fr/#faq"'),
    ('"name": "Do my friends need the mod?", "acceptedAnswer": { "@type": "Answer", "text": "Yes. Every player installs it, it is what sends the game to your server. It is one double-click." }',
     '"name": "Mes amis ont-ils besoin du mod ?", "acceptedAnswer": { "@type": "Answer", "text": "Oui. Chaque joueur l\'installe : c\'est lui qui envoie le jeu vers votre serveur. Un double-clic suffit." }'),
    ('"name": "Can I still play on the official servers?", "acceptedAnswer": { "@type": "Answer", "text": "Run the installer again and pick \\"Back to official multiplayer\\", or delete the three files." }',
     '"name": "Puis-je encore jouer sur les serveurs officiels ?", "acceptedAnswer": { "@type": "Answer", "text": "Relancez l\'installeur et choisissez « Revenir au multijoueur officiel », ou supprimez les trois fichiers." }'),
    ('"name": "Does it touch my saves or the game?", "acceptedAnswer": { "@type": "Answer", "text": "No. It sits next to the game and only redirects online play. Saves and game files stay as they are." }',
     '"name": "Ça touche à mes sauvegardes ou au jeu ?", "acceptedAnswer": { "@type": "Answer", "text": "Non. Il se pose à côté du jeu et ne fait que rediriger le jeu en ligne. Sauvegardes et fichiers du jeu restent intacts." }'),
    ('"name": "Do I need my own copy of the game?", "acceptedAnswer": { "@type": "Answer", "text": "Yes. pa-coop ships nothing from the game. Steam or Epic, every player needs a copy." }',
     '"name": "Faut-il posséder le jeu ?", "acceptedAnswer": { "@type": "Answer", "text": "Oui. pa-coop ne contient rien du jeu. Steam ou Epic, chaque joueur doit avoir sa copie." }'),
    ('"name": "What if a friend desyncs?", "acceptedAnswer": { "@type": "Answer", "text": "If the game detects it, the mod asks the host to resend the prison so the player can stay. A safety net, not a guarantee: some desyncs can still end the session." }',
     '"name": "Et si un ami se désynchronise ?", "acceptedAnswer": { "@type": "Answer", "text": "Si le jeu la détecte, le mod demande à l\'hôte de renvoyer la prison pour que le joueur reste. C\'est un filet de sécurité, pas une garantie : certaines désynchronisations peuvent encore mettre fin à la session." }'),
    ('"name": "Mods and the game\'s DLC?", "acceptedAnswer": { "@type": "Answer", "text": "Same rules as the official co-op: the game disables mods online, and players should own the same DLC." }',
     '"name": "Et les mods et les DLC ?", "acceptedAnswer": { "@type": "Answer", "text": "Mêmes règles que le co-op officiel : le jeu désactive les mods en ligne, et les joueurs doivent avoir les mêmes DLC." }'),
    # --- language switch and folder
    ('<nav class="lang-switch" aria-label="Language"><a href="/" aria-current="page">EN</a><a href="/fr/" hreflang="fr" lang="fr">FR</a></nav>',
     '<nav class="lang-switch" aria-label="Langue"><a href="/" hreflang="en" lang="en">EN</a><a href="/fr/" aria-current="page">FR</a></nav>'),
    ('Co-op for Prison Architect on your own server. Room for more than 4 players, Steam and Epic in the same game.',
     'Le co-op de Prison Architect sur votre propre serveur. De la place pour plus de 4 joueurs, Steam et Epic dans la même partie.'),
    ('aria-label="Main"', 'aria-label="Menu principal"'),
    ('<b>Download for Windows</b>', '<b>Télécharger pour Windows</b>'),
    ('<b>Run a server</b>', '<b>Héberger un serveur</b>'),
    ('<b>Source code</b>', '<b>Code source</b>'),
    ('works with The_Jailhouse_1.02', 'compatible The_Jailhouse_1.02'),
    ('aria-label="Screenshots"', 'aria-label="Captures d\'écran"'),
    ('alt="In-game player list showing 13 players in one prison"', 'alt="Liste des joueurs en jeu : 13 joueurs dans une même prison"'),
    ('<figcaption>13 players, one prison</figcaption>', '<figcaption>13 joueurs, une prison</figcaption>'),
    ('the game stops at 4. eleven of these are our test bots.', 'le jeu s\'arrête à 4. onze d\'entre eux sont nos bots de test.'),
    ('alt="The same prison on two computers side by side"', 'alt="La même prison sur deux ordinateurs côte à côte"'),
    ('<figcaption>Same prison, two PCs</figcaption>', '<figcaption>Même prison, deux PC</figcaption>'),
    # --- letter
    ('Office of the Warden', 'Bureau du directeur'),
    ('<p>To whoever runs this prison with friends,</p>', '<p>À celles et ceux qui dirigent cette prison entre amis,</p>'),
    ("<p>The online mode works, until it doesn't. Four players at most. Big prisons drift out of sync, and when the game notices, it shows an error and throws the player out. The servers it talks to belong to someone else, and the multiplayer hasn't been touched since 2023.</p>",
     "<p>Le mode en ligne marche, jusqu'au jour où il ne marche plus. Quatre joueurs au maximum. Les grosses prisons se désynchronisent, et quand le jeu s'en rend compte, il affiche une erreur et éjecte le joueur. Les serveurs qu'il utilise appartiennent à quelqu'un d'autre, et le multijoueur n'a pas bougé depuis 2023.</p>"),
    ("<p>pa-coop puts the server in your hands. It speaks the same language as the official one, so the game doesn't know the difference. The mod next to the game takes care of the rest: more players, and when the game reports a desync, the mod asks the host to resend the prison instead of dropping the player. It won't catch every case, but it is better than a kick.</p>",
     "<p>pa-coop vous met le serveur entre les mains. Il parle la même langue que l'officiel, le jeu ne voit pas la différence. Le mod posé à côté du jeu s'occupe du reste : plus de joueurs, et quand le jeu signale une désynchronisation, le mod demande à l'hôte de renvoyer la prison au lieu d'éjecter le joueur. Ça ne rattrape pas tout, mais c'est mieux qu'une éjection.</p>"),
    ('<p>Your saves and the game files stay as they are.</p>', '<p>Vos sauvegardes et les fichiers du jeu restent intacts.</p>'),
    ('<p class="signature marker">The Warden</p>', '<p class="signature marker">Le directeur</p>'),
    ('Tested on Ironfort: 380 prisoners, speed 4, two players building at once.<br><br>No desync in that run. Yours may vary.',
     'Testé sur Ironfort : 380 détenus, vitesse 4, deux joueurs qui construisent en même temps.<br><br>Aucune désync ce jour-là. Chez vous, à voir.'),
    # --- corkboard
    ('From the cameras', 'Vu par les caméras'),
    ('alt="The canteen at lunch, packed with prisoners"', 'alt="Le réfectoire à midi, plein de détenus"'),
    ('<figcaption>Lunch, 393 inmates. Same on every screen.</figcaption>', '<figcaption>Midi, 393 détenus. Pareil sur chaque écran.</figcaption>'),
    ('alt="Ironfort seen from above"', 'alt="Ironfort vue d\'en haut"'),
    ('<figcaption>Ironfort from above</figcaption>', '<figcaption>Ironfort vue d\'en haut</figcaption>'),
    ('alt="A cell block"', 'alt="Un bloc de cellules"'),
    ('<figcaption>Cell block B</figcaption>', '<figcaption>Bloc B</figcaption>'),
    ('alt="Free time in the yard"', 'alt="Temps libre dans la cour"'),
    ('<figcaption>Free time in the yard</figcaption>', '<figcaption>Temps libre dans la cour</figcaption>'),
    ('alt="Cells, shower block and morgue"', 'alt="Cellules, douches et morgue"'),
    ('<figcaption>Host on Epic, friend on Steam</figcaption>', '<figcaption>Hôte sur Epic, ami sur Steam</figcaption>'),
    ('All shots taken in the real game, during a co-op session through a pa-coop server.',
     'Toutes les captures viennent du vrai jeu, pendant une partie co-op sur un serveur pa-coop.'),
    # --- blueprint
    ('How it works', 'Comment ça marche'),
    ('Plan no. 1 · scale: not to scale', 'Plan n° 1 · échelle : aucune'),
    ('<h3>Your game (host)</h3><p>Prison Architect<br>+ the pa-coop mod<br>runs the simulation</p>',
     '<h3>Votre jeu (hôte)</h3><p>Prison Architect<br>+ le mod pa-coop<br>fait tourner la simulation</p>'),
    ("<h3>pa-coop server</h3><p>replaces Photon's cloud<br>one TCP port (4533)<br>lobby, game list,<br>message relay<br>up to 16 per game</p>",
     "<h3>Serveur pa-coop</h3><p>remplace le cloud Photon<br>un port TCP (4533)<br>salon, liste des parties,<br>relais des messages<br>jusqu'à 16 par partie</p>"),
    ('<h3>Friend on Steam</h3><p>same game, same mod<br>sees what you see</p>', '<h3>Ami sur Steam</h3><p>même jeu, même mod<br>voit ce que vous voyez</p>'),
    ('<h3>Friend on Epic</h3><p>Windows or Linux,<br>it does not matter</p>', '<h3>Ami sur Epic</h3><p>Windows ou Linux,<br>peu importe</p>'),
    ('one port to open', 'un seul port à ouvrir'),
    ('the mod points the game at your server instead of the official one', 'le mod envoie le jeu vers votre serveur au lieu de l\'officiel'),
    ('desync reported? the mod asks the host to resend the prison', 'désync signalée ? le mod demande à l\'hôte de renvoyer la prison'),
    # --- install
    ('<h2 class="marker">Install</h2>', '<h2 class="marker">Installation</h2>'),
    ('<h3>Windows (Steam or Epic)</h3><p>Download', '<h3>Windows (Steam ou Epic)</h3><p>Téléchargez'),
    (' from the latest release and double-click it. It finds the game, copies three files next to it and asks one question: host or join.</p>',
     ' depuis la dernière version et double-cliquez dessus. Il trouve le jeu, copie trois fichiers à côté et pose une seule question : héberger ou rejoindre.</p>'),
    ('<h3>Host</h3><p>Pick "Host a game". Load a prison, then Menu &gt; Go Online. The game starts the server for you and the router opens port 4533 (UPnP). The installer shows the address to give your friends.</p>',
     '<h3>Héberger</h3><p>Choisissez « Héberger une partie ». Chargez une prison, puis Menu &gt; Go Online. Le jeu lance le serveur tout seul et la box ouvre le port 4533 (UPnP). L\'installeur affiche l\'adresse à donner à vos amis.</p>'),
    ('<h3>Join</h3><p>Pick "Join a friend" and type their address. In the game: Join Online Game, their prison is in the list.</p>',
     '<h3>Rejoindre</h3><p>Choisissez « Rejoindre un ami » et tapez son adresse. Dans le jeu : Join Online Game, sa prison est dans la liste.</p>'),
    ('<h3>Linux (Steam)</h3><p>Same thing from a terminal, then add the launch option it prints:</p>',
     '<h3>Linux (Steam)</h3><p>Pareil depuis un terminal, puis ajoutez l\'option de lancement qu\'il affiche :</p>'),
    ("Router won't open the port? A VPN between friends (Tailscale, ZeroTier, NetBird) works too.",
     "La box n'ouvre pas le port ? Un VPN entre amis (Tailscale, ZeroTier, NetBird) marche aussi."),
    ('<p>The only setting, next to the game:</p>', '<p>Le seul réglage, à côté du jeu :</p>'),
    ('<span class="c"># host</span>', '<span class="c"># héberger</span>'),
    ('<span class="c"># join a friend</span>', '<span class="c"># rejoindre un ami</span>'),
    ("<span class=\"c\"># or pick in the game's</span>\n<span class=\"c\"># Region menu</span>", '<span class="c"># ou choisir dans le menu</span>\n<span class="c"># Region du jeu</span>'),
    ('servers=Us=pa.example.org', 'servers=Nous=pa.example.org'),
    # --- work order
    ('Work order — dedicated server', 'Bon de travaux — serveur dédié'),
    ('No. 0001', 'N° 0001'),
    ('Keep the prison online even when the host is asleep. One small program, one TCP port. Players set <code>server=&lt;your address&gt;</code>.',
     'La prison reste en ligne même quand l\'hôte dort. Un petit programme, un port TCP. Les joueurs mettent <code>server=&lt;votre adresse&gt;</code>.'),
    ('<p>Image on GitHub Container Registry, amd64 and arm64, 6 MB.</p>', '<p>Image sur GitHub Container Registry, amd64 et arm64, 6 Mo.</p>'),
    ('<h3>Plain binary</h3><p>Linux (amd64, arm64) or Windows, from the <a', '<h3>Binaire</h3><p>Linux (amd64, arm64) ou Windows, depuis la <a'),
    ('>release page</a>.</p>', '>page des versions</a>.</p>'),
    ('<p>Import the egg, create a server with one port allocation, start. Reinstall to update.</p>',
     '<p>Importez l\'egg, créez un serveur avec une allocation de port, démarrez. Réinstallez pour mettre à jour.</p>'),
    ('<small>Approved by: the Warden</small>', '<small>Validé par : le directeur</small>'),
    # --- incidents
    ('Incident reports', 'Rapports d\'incident'),
    ('Questions people ask, filed and answered.', 'Les questions qu\'on nous pose, classées et répondues.'),
    ('<h3>Do my friends need the mod?</h3><p>Yes. Every player installs it, it is what sends the game to your server. It is one double-click.</p>',
     '<h3>Mes amis ont-ils besoin du mod ?</h3><p>Oui. Chaque joueur l\'installe : c\'est lui qui envoie le jeu vers votre serveur. Un double-clic suffit.</p>'),
    ('<h3>Can I still play on the official servers?</h3><p>Run the installer again and pick "Back to official multiplayer", or delete the three files.</p>',
     '<h3>Puis-je encore jouer sur les serveurs officiels ?</h3><p>Relancez l\'installeur et choisissez « Revenir au multijoueur officiel », ou supprimez les trois fichiers.</p>'),
    ('<h3>Does it touch my saves or the game?</h3><p>No. It sits next to the game and only redirects online play. Saves and game files stay as they are.</p>',
     '<h3>Ça touche à mes sauvegardes ou au jeu ?</h3><p>Non. Il se pose à côté du jeu et ne fait que rediriger le jeu en ligne. Sauvegardes et fichiers du jeu restent intacts.</p>'),
    ('<h3>Do I need my own copy of the game?</h3><p>Yes. pa-coop ships nothing from the game. Steam or Epic, every player needs a copy.</p>',
     '<h3>Faut-il posséder le jeu ?</h3><p>Oui. pa-coop ne contient rien du jeu. Steam ou Epic, chaque joueur doit avoir sa copie.</p>'),
    ('<h3>What if a friend desyncs?</h3><p>If the game detects it, the mod asks the host to resend the prison so the player can stay. A safety net, not a guarantee: some desyncs can still end the session.</p>',
     '<h3>Et si un ami se désynchronise ?</h3><p>Si le jeu la détecte, le mod demande à l\'hôte de renvoyer la prison pour que le joueur reste. C\'est un filet de sécurité, pas une garantie : certaines désynchronisations peuvent encore mettre fin à la session.</p>'),
    ("<h3>Mods and the game's DLC?</h3><p>Same rules as the official co-op: the game disables mods online, and players should own the same DLC.</p>",
     "<h3>Et les mods et les DLC ?</h3><p>Mêmes règles que le co-op officiel : le jeu désactive les mods en ligne, et les joueurs doivent avoir les mêmes DLC.</p>"),
    # --- release
    ('<h2>Release form</h2>', '<h2>Bon de sortie</h2>'),
    ('MIT licence ·', 'licence MIT ·'),
    ('<span>Windows installer, start here</span>', '<span>installeur Windows, commencez ici</span>'),
    ('<span>the mod and server, to copy by hand</span>', '<span>le mod et le serveur, à copier à la main</span>'),
    ('<span>Linux (Steam, native version)</span>', '<span>Linux (Steam, version native)</span>'),
    ('<span>dedicated server</span>', '<span>serveur dédié</span>'),
    ('<span>dedicated server on Windows</span>', '<span>serveur dédié sous Windows</span>'),
    ('<span>Docker image</span>', '<span>image Docker</span>'),
    ('Released. Report problems on <a', 'Publié. Signalez les problèmes sur <a'),
    # --- footer
    ('Independent open-source project (MIT). Not affiliated with Paradox Interactive, Double Eleven, Introversion or Exit Games. Prison Architect is a trademark of Paradox Interactive; screenshots taken in the game.',
     'Projet open source indépendant (MIT), sans lien avec Paradox Interactive, Double Eleven, Introversion ou Exit Games. Prison Architect est une marque de Paradox Interactive ; captures prises dans le jeu.'),
    ('<a href="/legal">Legal notice and privacy</a>', '<a href="/fr/legal">Mentions légales et confidentialité</a>'),
    ('<a href="/fr/" hreflang="fr" lang="fr">Version française</a>', '<a href="/" hreflang="en" lang="en">English version</a>'),
]


def translate(src, pairs, name):
    out, missing = src, []
    for en, fr in pairs:
        if en not in out:
            missing.append(en[:90])
        out = out.replace(en, fr)
    if missing:
        sys.exit(f"{name}: strings not found in the English page:\n  - " + "\n  - ".join(missing))
    return out


def main():
    os.makedirs(os.path.join(D, "fr"), exist_ok=True)
    with open(os.path.join(D, "index.html"), encoding="utf-8") as f:
        en = f.read()
    fr = translate(en, INDEX, "index")
    # the fragment links stay valid; checks no English heading is left behind
    for left in ("Download for Windows", "Office of the Warden", "How it works", "Release form"):
        assert left not in fr, left
    with open(os.path.join(D, "fr", "index.html"), "w", encoding="utf-8") as f:
        f.write(fr)
    print("fr/index.html written")


if __name__ == "__main__":
    main()
