---
title: Fonctionnalités
linkTitle: Fonctionnalités
weight: 10
description: >
  Ce que HttpStackLens sait faire aujourd'hui, et comment chaque brique s'articule.
---

## 🔀 Proxy direct & tunneling CONNECT

À la base, HttpStackLens est un proxy direct (forward proxy). Il écoute les
connexions entrantes sur un port local (par défaut `3128`), gère les tunnels HTTPS
via la méthode `CONNECT` et relaie requêtes et réponses de façon bidirectionnelle.
Il analyse les corps de réponse en `chunked` comme en `Content-Length`.

```sh
curl -x http://localhost:3128 http://example.com
```

![La liste des requêtes en direct, les plus récentes en premier.](/images/screenshots/request-list.png)

## 🔓 Interception & déchiffrement HTTPS

Lorsque `decrypt_https.enabled` est activé, HttpStackLens réalise un MITM local
optionnel. Un gestionnaire de certificats intégré génère un **CA racine** de
débogage, l'installe dans le magasin Windows / le trousseau macOS, et émet un
**certificat par domaine** pour chaque hôte visité. Le déchiffrement se bascule à
chaud depuis l'interface — aucun redémarrage requis.

Des règles par type MIME dans la config décident quels corps sont capturés et
jusqu'à quelle taille, pour éviter de bufferiser un téléchargement vidéo de 4 Go
juste pour lire un JSON.

{{% alert title="🔐 Sûr par conception" color="info" %}}
Chaque certificat créé par l'application porte un marqueur distinctif dans son
sujet (`My Local CA for debugging HTTPS`). Le nettoyage se base sur ce marqueur, il
ne retire donc que les certificats installés par l'application elle-même. Voir le
[tutoriel de déchiffrement HTTPS](../tutorial-https-decrypt/).
{{% /alert %}}

![Une réponse HTTPS déchiffrée, en-têtes et corps en clair.](/images/screenshots/decrypted-body.png)

## 📡 Interface web temps réel

Une interface WASM + Tailwind diffuse le trafic via Server-Sent Events. Elle offre
une liste des requêtes (les plus récentes en premier), l'inspection détaillée
requête/réponse avec timings, le décodage base64 des corps avec aperçu d'images en
ligne, et un panneau de détail redimensionnable et persistant, en thème clair ou
sombre.

L'interface fait aussi office de panneau de contrôle : démarrer/arrêter le proxy,
gérer l'enregistrement, et modifier les réglages du proxy amont, de la capture des
corps et du contrôle d'accès — sans toucher au fichier de config ni redémarrer le
binaire.

![Requête et réponse côte à côte, avec bascule de focus.](/images/screenshots/split-panes.png)

## 🏢 Authentification du proxy amont

HttpStackLens peut tout relayer vers un autre proxy défini par `output_proxy_uri`.
Sous Windows, il peut injecter les identifiants de la session en cours via NTLM,
Kerberos ou Negotiate (`add_windows_authentication_to_output_proxy`), et embarque un
mode de compatibilité pour les proxys amont qui s'authentifient par des défis
`407`/`401`.

Cette combinaison lui permet de remplacer un assistant d'authentification local
dédié sur un poste de dev situé derrière un proxy d'entreprise authentifié — voir le
[tutoriel dédié au proxy d'entreprise](../tutorial-upstream-proxy/).

![Le flux de compatibilité 401/407 du proxy amont.](/images/upstream-proxy-401-compatibility-flow.png)

## 💾 Capture & stockage du trafic

Enregistrez des sessions dans un format binaire `.capture`, parcourez les captures
sauvegardées dans l'interface et interrogez le trafic via une API REST
(`/api/requests/…`). Un tampon mémoire borné conserve les requêtes les plus récentes
même quand le stockage est désactivé.

![La structure du datagramme .capture sur disque.](/images/capture-file-format-datagram.png)

## 🛡️ Contrôle d'accès

Les connexions distantes sont restreintes par défaut. Le proxy comme l'interface web
supportent les modes `loopback`, LAN et liste d'autorisation, pour décider exactement
qui peut les atteindre.

| Mode        | Qui peut se connecter                                       |
|-------------|-------------------------------------------------------------|
| `loopback`  | Uniquement cette machine (127.0.0.1 / ::1). Le défaut.      |
| `lan`       | Les plages d'adresses privées / LAN.                        |
| `allowlist` | Uniquement les réseaux précis listés sous `networks`.       |

{{% alert title="📱 Déboguer un container ou un appareil mobile" color="info" %}}
C'est là que `allowlist` brille. Pour inspecter le trafic d'une application qui
tourne dans un **container Docker** ou sur un **téléphone / une tablette**, ce client
doit joindre le proxy depuis une adresse non-loopback — mais l'ouvrir à tout le LAN
est excessif. À la place, autorisez uniquement la source qui vous intéresse : le
réseau bridge du container (ex. `172.17.0.0/16`) ou l'IP de votre appareil (ex.
`192.168.1.42/32`). Pointez ensuite le container / l'appareil vers l'IP de votre
machine sur `:3128` et observez ses requêtes en direct.
{{% /alert %}}

{{% alert title="🔧 Deux contrôles d'accès distincts" color="warning" %}}
Le **proxy** et l'**interface web** ont chacun leur propre bloc `access_control`
dans `config.yaml` (`proxy.access_control` et `webui.access_control`) — ils se
configurent indépendamment. En ouvrir un ne touche pas l'autre : si vous voulez à la
fois envoyer du trafic *et* ouvrir l'UI depuis un autre appareil, réglez le mode sur
les **deux**. Une configuration courante est un `allowlist` sur le proxy pendant que
l'UI reste en `loopback`.
{{% /alert %}}

## ⚙️ Configuration & mises à jour

Un unique `config.yaml` avec des valeurs par défaut sensées est généré au premier
lancement. Le binaire indique sa version en cours avec un lien vers son commit, et
peut optionnellement vérifier une nouvelle version sur GitHub au démarrage (opt-in
via `updates.check_enabled`).

```yaml
proxy:
  port: 3128
  output_proxy_uri:
  add_windows_authentication_to_output_proxy: false
webui:
  port: 9000
decrypt_https:
  enabled: false  # intercepte & déchiffre le HTTPS (génère + installe un certificat par domaine)
```
