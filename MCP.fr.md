# Feuille de route MCP pour HttpStackLens

L’objectif est de permettre à un assistant de comprendre le trafic observé par
HttpStackLens, puis de piloter une séance de diagnostic et de préparer des
reproductions. Le serveur fournit les faits et les opérations ; le modèle reste
côté client, par exemple dans LM Studio.

Les cases cochées correspondent aux fonctionnalités MCP déjà implémentées.
Les cases ouvertes sont des fonctionnalités à construire, même lorsqu’un service
ou une API Web UI existe déjà. Les noms d’outils proposés restent indicatifs.

## 1. Prototype et connexion

- [x] Serveur MCP utilisant le SDK Go et le transport Streamable HTTP.
- [x] Listener local distinct du proxy et de la Web UI, sur le port 8087 par défaut.
- [x] Activation explicite par `mcp.enabled` dans `config.yaml`.
- [x] Validation du port et avertissement en cas d’échec, sans arrêter le proxy.
- [x] Fermeture du listener à la sortie de l’application.
- [x] Outil de démonstration `cityTime`.
- [x] Tests de connexion MCP et de découverte des outils avec le client SDK Go.
- [ ] Valider le parcours complet avec LM Studio : connexion, découverte et appel d’un outil.
- [ ] Afficher dans la Web UI l’état MCP, son URL et une éventuelle erreur de démarrage.
- [ ] Rendre visible le chemin du fichier de configuration réellement chargé et indiquer les réglages nécessitant un redémarrage.
- [ ] Fournir un exemple de configuration LM Studio copiable depuis la Web UI.
- [ ] Retirer `cityTime` lorsque les premiers outils utiles au proxy sont disponibles.

Configuration actuelle :

```yaml
mcp:
  enabled: true
  port: 8087
```

Le client se connecte à `http://127.0.0.1:8087/`. Il faut démarrer HttpStackLens
avant la connexion et redémarrer l’application après modification de ces réglages.
Sans section `mcp`, le serveur reste désactivé.

**Attention avec GoLand :** `config.yaml` est résolu à côté de l’exécutable,
pas dans le répertoire de travail. Un lancement depuis l’IDE peut donc utiliser
un fichier situé dans son dossier temporaire. Le chemin chargé figure déjà dans
les logs de démarrage.

Le listener séparé est l’architecture retenue pour cette feuille de route.
Le flux SSE `/events` de la Web UI reste indépendant du protocole MCP.

## 2. Priorité 1 — Comprendre le trafic récent

Parcours cible : « J’ai relancé `npm install`, retrouve les échanges en échec et
aide-moi à comprendre pourquoi. »

- [x] `get_status` : lire l’état du proxy, de l’enregistrement, de la persistance, de l’upstream et du MITM, sans exposer de secrets.
- [x] `search_requests` : rechercher les échanges par période, hôte, méthode, chemin, statut HTTP et présence d’une réponse. Le filtre sur les erreurs réseau détaillées reste à construire.
- [ ] Inclure les échecs sans réponse HTTP : connexion refusée, timeout ou erreur TLS, lorsque ces informations sont disponibles.
- [ ] Retourner des listes compactes : identifiant, date, méthode, URL masquée, statut ou erreur, durée et mode HTTPS.
- [ ] Paginer les résultats avec une limite bornée et un curseur ; définir le comportement lorsque le trafic continue d’arriver.
- [ ] `get_request` : lire les détails d’un échange, ses en-têtes filtrés et les mesures de temps disponibles.
- [ ] Distinguer un échange terminé, encore en cours, introuvable ou évincé de la mémoire lorsque le suivi disponible le permet.
- [ ] Expliquer les limites de visibilité : enregistrement arrêté, tunnel CONNECT opaque, corps non capturé ou données tronquées.
- [ ] Fournir les identifiants des échanges qui justifient chaque résultat pour permettre leur consultation dans l’inspecteur.

Un statut `407` du proxy upstream doit rester distinct d’un `401` du serveur cible.
Les durées ne doivent pas être attribuées au DNS, au TLS ou à l’upstream sans
mesure correspondante. L’identification du processus client et les détails SSPI
ne doivent pas être promis lorsqu’ils ne sont pas collectés.

## 3. Priorité 2 — Examiner les contenus et résumer une session

- [ ] `get_body_excerpt` : lire un extrait borné du corps de requête ou de réponse, sur demande explicite du client.
- [ ] Indiquer le type de contenu, la taille disponible, le masquage appliqué et une éventuelle troncature.
- [ ] Distinguer corps vide, absent, ignoré par les règles de capture et inaccessible en tunnel HTTPS.
- [ ] Définir les formats de corps pris en charge et éviter de retourner des données binaires brutes au modèle.
- [ ] `summarize_traffic` : calculer les volumes, erreurs et durées par destination sur un périmètre explicite.
- [ ] Indiquer si les agrégats couvrent toute la période demandée ou seulement les données encore disponibles.
- [ ] `compare_requests` : comparer deux échanges et retourner les différences de méthode, URL, en-têtes, statut et contenu disponible.

Les listes n’incluent pas les corps par défaut. Le modèle peut approfondir quelques
échanges après une recherche, ce qui limite la quantité de contexte consommée.
La rédaction d’une explication ou d’un rapport peut rester à la charge du modèle.

## 4. Priorité 3 — Piloter les sessions de capture

Parcours cible : « Commence une capture pour mon problème de connexion, je le
reproduis, puis arrête et conserve la session pour l’analyser. »

Trois états doivent rester distincts : le listener du proxy, l’enregistrement du
trafic dans l’inspecteur et la persistance des données dans des fichiers `.capture`.
La notion de session nommée et son cycle de vie restent à construire.

### Enregistrement et périmètre

- [ ] `get_capture_state` : lire les états d’enregistrement et de persistance, la session active et les compteurs disponibles.
- [ ] `start_capture` : démarrer une session nommée, avec un identifiant stable et une heure de début.
- [ ] Définir le comportement si une capture est déjà active : retourner son état ou signaler un conflit, sans remplacer silencieusement la session.
- [ ] `stop_capture` : arrêter l’enregistrement et retourner un bilan avec les identifiants des fichiers effectivement produits.
- [ ] Définir le traitement des requêtes encore en cours à l’arrêt et signaler les échanges incomplets.
- [ ] `pause_capture` / `resume_capture` : suspendre puis reprendre une session en conservant ses limites temporelles et en signalant les périodes non enregistrées.
- [ ] Permettre un périmètre par hôte ou chemin ; préciser s’il filtre réellement l’enregistrement ou uniquement les résultats d’analyse.
- [ ] Prévoir une limite de durée, de volume ou de nombre d’échanges, avec une raison d’arrêt explicite.
- [ ] `add_capture_marker` : placer un repère nommé, par exemple « début npm install » ou « erreur affichée », sans modifier les échanges.
- [ ] `read_capture_updates` : lire les nouveaux échanges avec curseur et attente bornée, en signalant les données perdues ou expirées.

### Sauvegarde et gestion des sessions

- [ ] `set_capture_persistence` : activer ou désactiver l’écriture sur disque et persister le réglage correspondant dans `config.yaml`.
- [ ] `save_capture_snapshot` : sauvegarder les échanges encore disponibles en mémoire dans une nouvelle capture, sans écraser un fichier existant.
- [ ] Indiquer qu’une sauvegarde a posteriori ne récupère pas les échanges évincés ni les corps jamais capturés.
- [ ] Associer une session à ses métadonnées : nom, dates, marqueurs, périmètre, fichiers et raison d’arrêt.
- [ ] `rename_capture` : renommer une capture ou sa description, en refusant les collisions.
- [ ] `clear_live_capture` : vider le tampon de l’inspecteur sans supprimer les fichiers sauvegardés ; invalider explicitement les curseurs concernés.
- [ ] `delete_capture` : supprimer uniquement une capture désignée, avec un contrôle explicite des actions destructives et une protection des fichiers encore utilisés.

Arrêter ou mettre en pause une capture ne coupe pas le forwarding. Désactiver
la persistance ne doit pas être présenté comme un arrêt de l’enregistrement.
Les opérations sur une session sont partagées avec la Web UI : plusieurs clients
MCP doivent observer le même état, et les transitions concurrentes doivent être
gérées de façon prévisible.

## 5. Priorité 4 — Analyser les captures sauvegardées

- [ ] `list_captures` : lister les captures disponibles avec leurs métadonnées, sans charger tout leur contenu.
- [ ] `get_capture_metadata` : lire le périmètre, les dates et les informations effectivement conservées.
- [ ] `read_capture_records` : lire les échanges d’une capture par pages bornées.
- [ ] Réutiliser les filtres de recherche et les résumés pour les captures sauvegardées.
- [ ] Préciser la source des résultats : trafic en mémoire ou fichier de capture identifié.
- [ ] `compare_captures` : comparer deux séances, avec des règles explicites de rapprochement des requêtes.
- [ ] Signaler les fichiers incomplets, illisibles ou supprimés pendant la consultation.
- [ ] Exporter une sélection en HAR avec une politique explicite de masquage ; cette conversion reste une fonctionnalité distincte.

La lecture des fichiers existants peut être livrée indépendamment du pilotage
des sessions nommées. Les outils doivent utiliser des noms ou identifiants validés
dans le dossier de captures, sans accepter de chemin arbitraire.

## 6. Ensuite — Préparer et exécuter des reproductions

- [ ] `prepare_request` : reconstruire une requête à partir d’un échange et proposer des modifications sans effectuer d’appel réseau.
- [ ] Signaler les corps manquants et remplacer les secrets masqués par des paramètres explicites, sans inventer leur valeur.
- [ ] Produire un fichier `.http` consultable et modifiable dans le Composer.
- [ ] `send_request` : envoyer une requête via la pipeline existante, avec un contrôle distinct des outils de lecture.
- [ ] Retourner le statut de destination, les erreurs réseau et l’identifiant du nouvel échange observé.
- [ ] Comparer la reproduction à l’échange initial.

Le Composer peut envoyer une requête même lorsque le listener TCP du proxy est
arrêté. Un HTTP 200 de l’API Composer ne garantit pas le succès de la destination :
le résultat MCP doit restituer également le champ d’erreur et le statut observé.

## 7. Ensuite — Réglages et diagnostics avancés

- [ ] Lire les informations publiques de la CA et expliquer le mode tunnel ou MITM actif.
- [ ] `update_upstream_settings` : modifier les réglages ciblés de l’upstream en réutilisant leur validation et leur persistance.
- [ ] Distinguer la configuration enregistrée de celle appliquée, et indiquer un éventuel redémarrage nécessaire.
- [ ] Modifier les limites de capture des corps avec persistance des réglages configurables.
- [ ] Contrôler explicitement l’activation du MITM ; elle ne doit jamais découler automatiquement d’un diagnostic.
- [ ] Ajouter éventuellement des ressources MCP pour l’état ou une capture identifiée.
- [ ] Ajouter éventuellement des prompts de diagnostic réutilisables.

L’installation de certificats, la configuration automatique du proxy système et
les fonctions d’administration multi-utilisateur restent hors du premier périmètre.

## 8. Exigences communes aux fonctionnalités

- [ ] Partager les services Go avec la Web UI, sans dupliquer le forwarding, l’authentification Windows ou la persistance.
- [ ] Commencer avec les outils de lecture ; contrôler séparément capture, écriture, rejeu, réglages et suppression.
- [ ] Masquer côté serveur les en-têtes sensibles, cookies et paramètres d’URL concernés avant transmission au modèle.
- [ ] Définir des règles configurables pour les secrets dans les corps ; ne pas promettre un masquage automatique exhaustif.
- [ ] Borner les tailles, pages et temps d’exécution pour préserver la réactivité du proxy.
- [ ] Retourner des résultats structurés avec provenance, limites et erreurs compréhensibles.
- [ ] Traiter les contenus capturés comme des données non fiables, jamais comme des instructions autorisant une action.
- [ ] Vérifier la validation d’`Origin` et les protections du listener local ; définir l’authentification adaptée aux clients retenus.
- [ ] Mettre à jour `openapi.yaml` dans toute modification des endpoints ou DTO de la Web UI.
- [ ] Persister dans `config.yaml` toute modification d’un réglage qui y est configuré, en réutilisant les mécanismes existants.
- [ ] Documenter et tester les schémas des outils MCP, complémentaires du contrat OpenAPI.

L’accès MCP reste local, mais les données peuvent être transmises par le client
à un modèle distant. La connexion MCP ne configure pas le proxy des outils et
ne rend pas visibles les contenus HTTPS restés dans un tunnel opaque.

## 9. Validation et critères de livraison

- [ ] Depuis LM Studio, retrouver une requête en échec et consulter les données qui permettent de l’expliquer.
- [ ] Vérifier la distinction entre erreurs HTTP, erreurs réseau et erreurs d’exécution de l’outil MCP.
- [ ] Vérifier pagination, éviction mémoire et reprise par curseur pendant l’arrivée de nouveaux échanges.
- [ ] Vérifier le masquage, la troncature et les différents motifs d’absence de corps.
- [ ] Démarrer, suspendre, reprendre et arrêter une session sans interrompre le trafic du proxy.
- [ ] Retrouver une session sauvegardée après redémarrage et signaler correctement les données incomplètes.
- [ ] Vérifier les conflits entre actions Web UI et clients MCP concurrents.
- [ ] Vérifier qu’un client MCP lent ou déconnecté ne bloque pas le forwarding.
- [ ] Vérifier que les actions d’écriture ou destructives sont refusées lorsqu’elles ne sont pas autorisées.
- [ ] Tester les scénarios HTTP, CONNECT opaque et MITM explicitement activé ; compléter sur Windows pour l’authentification upstream.

Le premier jalon livrable est volontairement limité à `get_status`,
`search_requests` et `get_request`, avec masquage, pagination et validation dans
LM Studio. Les fonctionnalités de pilotage et de sauvegarde de sessions viennent
ensuite sans retarder ce parcours initial.

## Références

- [Contrat HTTP de la Web UI](webui/wwwroot/openapi.yaml)
- [Handlers de la Web UI](webui/ui_server.go)
- [Prototype MCP](mcp/mcp_http_server.go)
- [Transport MCP Streamable HTTP](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
- [Outils MCP](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)

## Contrat actuel de `get_status`

`get_status` ne prend aucun argument (`{}`). Cet outil en lecture seule renvoie
un résultat structuré et sa représentation textuelle pour les clients MCP.
Il lit les contrôleurs partagés avec la Web UI à chaque appel.

| Champ | Signification |
| --- | --- |
| `proxy.running`, `proxy.address` | État et adresse du listener ; son arrêt n’interdit pas les envois du Composer. |
| `capture.recording` | Enregistrement des nouveaux échanges activé. |
| `capture.persistence_enabled` | Fichier de capture ouvert ; l’enregistrement doit également être actif pour capturer le nouveau trafic. |
| `capture.buffered_exchanges` | Nombre d’échanges actuellement conservés en mémoire. |
| `https.decryption_enabled` | MITM activé pour les nouveaux CONNECT ; les tunnels existants gardent leur mode initial. |
| `upstream.enabled` | Un proxy upstream est configuré. |
| `upstream.windows_authentication_configured` | Option d’authentification Windows configurée. |
| `upstream.windows_authentication_applicable` | Option configurée avec un upstream sur Windows ; ne prouve pas une authentification réussie. |

Ce résultat ne contient ni URL upstream, ni identifiants, ni en-têtes, ni corps,
ni chemins de fichiers. Il ne teste pas la connectivité et n’atteste pas que le
navigateur utilise le proxy. Les champs sont lus dans des stores synchronisés,
mais ne constituent pas une transaction atomique entre tous les contrôleurs.
Si les dépendances runtime sont absentes, l’outil retourne une erreur explicite.
Le contrat HTTP de la Web UI est inchangé.

## Contrat actuel de `search_requests`

Outil en lecture seule, limité aux échanges encore en mémoire. Aucun endpoint
Web UI ni format de capture sur disque n’est modifié.

Arguments facultatifs :

| Argument | Comportement |
| --- | --- |
| `host` | Nom d’hôte exact, sans port, insensible à la casse ; pas d’inclusion automatique des sous-domaines. |
| `method` | Méthode HTTP, insensible à la casse. |
| `path_contains` | Sous-chaîne du chemin décodé, sensible à la casse ; ne recherche pas dans la query. |
| `status_code` | Statut exact entre 100 et 599 ; absent ou zéro pour tous les statuts. |
| `has_response` | Présence d’une réponse enregistrée. `false` ne prouve pas une erreur réseau. |
| `since`, `until` | Bornes inclusives RFC3339 sur la première observation de l’échange dans le store. |
| `limit` | 25 par défaut (également pour zéro), maximum 100. |
| `cursor` | Valeur de `next_cursor` de la page précédente, en répétant les mêmes filtres. |

Exemple :

```json
{"host":"example.com","method":"GET","status_code":503,"limit":25}
```

Le résultat contient `source: "memory"`, un tableau `requests`, les `limitations`
et éventuellement `next_cursor`. Chaque résumé expose l’identifiant, la date de
première observation, la méthode, l’URL masquée, le statut et la durée mesurée
(ou `null`), la présence de chaque côté de l’échange et sa visibilité.
`connect` identifie un échange CONNECT, sans conclure au mode des requêtes internes ;
`https_request` et `http_request` reposent sur le schéma enregistré. En l’absence
de cette information, la visibilité vaut `unknown`.

Les résultats sont triés par ordre d’insertion, du plus récent au plus ancien.
La pagination exclut les insertions postérieures à la première page. Les réponses
et timings restent vivants et peuvent évoluer entre les pages : il ne s’agit pas
d’une photographie figée. Une éviction, un effacement ou un changement de store
invalide les curseurs ; le client doit alors relancer une recherche sans curseur.
Un curseur invalide ou des filtres différents provoquent une erreur explicite.

Les en-têtes, corps et messages de réponse sont exclus. Les informations utilisateur
et fragments d’URL sont supprimés, et toute la query est remplacée par `REDACTED`.
Une URL invalide est omise ; les URL longues sont tronquées et signalées.
Les chemins restent visibles et peuvent contenir des données sensibles : cette
première politique ne garantit pas un masquage exhaustif. Le store original n’est
pas modifié. Les erreurs réseau détaillées ne sont pas actuellement conservées
par ce store ; une absence de réponse peut aussi correspondre à un échange en cours.
