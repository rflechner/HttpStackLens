# Ce qu’un serveur MCP HTTP pourrait apporter à HttpStackLens

Ce document propose des usages et un périmètre pour une future intégration MCP. Les noms d’outils et l’adresse `/mcp` ci-dessous sont des propositions, pas des fonctionnalités déjà disponibles.

L’objectif serait de permettre à un assistant IA de consulter le trafic observé par HttpStackLens, d’aider à comprendre un problème réseau et, lorsque l’utilisateur le demande, de préparer ou d’exécuter certaines actions de diagnostic.

## 1. Le principe

MCP (Model Context Protocol) permet à une application compatible de découvrir et d’utiliser des outils, des ressources et des modèles de prompts fournis par un serveur. HttpStackLens pourrait ainsi devenir une source de contexte et d’actions pour un assistant de développement. Voir les spécifications des [outils](https://modelcontextprotocol.io/specification/2025-11-25/server/tools), des [ressources](https://modelcontextprotocol.io/specification/2025-11-25/server/resources) et des [prompts](https://modelcontextprotocol.io/specification/2025-11-25/server/prompts).

Le serveur MCP ne contiendrait pas nécessairement de modèle IA : il fournirait des données structurées et des opérations déterministes. L’assistant connecté interpréterait les résultats et expliquerait ses conclusions à l’utilisateur.

```text
curl / npm / NuGet / Git / application
                   |
                   v
          Proxy HttpStackLens ------> Proxy d’entreprise / destination
                   |
          Captures et état local
                   |
          Serveur MCP HTTP <--------> Client MCP de l’assistant
                   |
           Services partagés
           avec la Web UI
```

Le trafic à inspecter doit toujours passer par HttpStackLens. Une connexion MCP ne configure pas automatiquement le proxy de npm, de Git ou du système.

## 2. Pourquoi un transport HTTP ?

Un point d’entrée local, par exemple `http://127.0.0.1:9000/mcp`, permettrait à un client compatible de se connecter à l’instance déjà démarrée. Plusieurs clients pourraient consulter le même état, sous réserve de gérer leurs accès et leurs actions concurrentes.

Le transport à viser serait **Streamable HTTP**, qui transporte des messages MCP en JSON-RPC et peut utiliser SSE. Le flux Web UI `/events` existant ne constitue pas à lui seul un serveur MCP : il faudrait ajouter la couche protocolaire. La [spécification du transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) fournit une référence versionnée ; la version effectivement implémentée devra être choisie selon les clients ciblés.

Cette adresse serait un point d’entrée de contrôle sur le serveur Web UI, distinct du port du proxy auquel les outils envoient leur trafic. Un client exécuté uniquement dans le cloud ne peut pas joindre directement le `127.0.0.1` de la machine du développeur. Le périmètre proposé reste un usage local, avec un client pouvant accéder à cette machine.

## 3. Usages concrets

### Comprendre un échec derrière un proxy d’entreprise

Exemple de demande : « J’ai relancé `npm install`. Peux-tu regarder pourquoi les téléchargements échouent ? »

L’assistant pourrait consulter les échanges sur la période concernée, regrouper les erreurs par destination et rapprocher les observations des réglages upstream et d’authentification Windows. Il pourrait distinguer un `407 Proxy Authentication Required`, un `401` de la destination, une erreur TLS ou un délai d’attente lorsque ces informations sont effectivement disponibles.

Le résultat attendu serait une explication accompagnée des identifiants des échanges pertinents et d’une prochaine vérification concrète. Il faudrait distinguer les faits observés des hypothèses : une capture ne garantit pas la visibilité sur chaque étape interne de SSPI, ni l’identification du processus à l’origine d’une requête.

### Explorer une session de trafic

Exemples : « Liste les réponses 5xx des cinq dernières minutes », « Quelles destinations sont les plus lentes ? », « Compare ces deux appels à mon API ».

Le serveur pourrait filtrer les échanges, calculer des agrégats et retourner un résumé compact. L’assistant pourrait ensuite demander les en-têtes ou un extrait de corps pour quelques échanges seulement. La recherche, les agrégations et la comparaison seraient des fonctions à ajouter autour des données existantes.

Les durées devraient être présentées selon les mesures réellement collectées. Une durée totale ne suffit pas à conclure que le DNS, la négociation TLS ou le serveur distant est responsable.

### Expliquer les limites de visibilité HTTPS

Exemple : « Pourquoi je vois un CONNECT mais pas les appels HTTPS à l’intérieur ? »

L’assistant pourrait lire l’état du déchiffrement et les informations de la CA locale, puis expliquer les deux modes :

- **Tunnel CONNECT** : le contenu TLS reste opaque ; MCP ne rend pas accessibles les URL internes, en-têtes ou corps chiffrés.
- **Inspection MITM activée explicitement** : les échanges déchiffrés peuvent être consultés selon les règles de capture et les données effectivement conservées.

Activer le MITM ne déchiffre pas rétroactivement les anciens tunnels. Le réglage actuel s’applique aux nouveaux CONNECT. Un corps ignoré par les limites de capture ne peut pas être reconstitué par l’assistant.

### Préparer une reproduction avec le Composer

Exemple : « Prépare une variante de cette requête avec un autre en-tête Accept ».

L’assistant pourrait reconstruire une requête à partir des éléments capturés, présenter les modifications et produire un fichier `.http`. À la demande de l’utilisateur, il pourrait l’envoyer via le Composer, qui utilise déjà la pipeline du proxy : upstream, règles de bypass et mode HTTPS.

Un rejeu à partir d’une capture demanderait une logique supplémentaire ; l’API d’envoi existe déjà. La reproduction peut être incomplète si le corps manque ou si des secrets ont été masqués. L’assistant devrait le signaler et ne pas inventer les valeurs absentes.

L’envoi constitue une véritable action réseau, susceptible de modifier des données côté destination. Il doit être séparé de la simple préparation. Le Composer fonctionne même lorsque le listener TCP du proxy est arrêté ; arrêter ce listener ne doit donc pas être considéré comme une interdiction d’envoyer des requêtes.

### Analyser une capture enregistrée

Exemple : « Analyse cette capture et prépare un compte rendu des erreurs ».

L’assistant pourrait parcourir un fichier `.capture` par pages, sélectionner les échanges intéressants et produire un rapport : destinations, erreurs, chronologie disponible et pistes de reproduction. Il pourrait aussi comparer deux captures, à condition d’ajouter les règles de rapprochement et les calculs nécessaires.

Le serveur fournirait les faits et références ; la rédaction du rapport pourrait rester à la charge de l’assistant. Un export HAR serait une fonctionnalité distincte à développer.

### Piloter une séance de diagnostic

Exemple : « Démarre l’enregistrement, je reproduis le problème, puis on examine les erreurs ».

Le MCP pourrait exposer le démarrage et l’arrêt de l’enregistrement, puis permettre une lecture des nouveaux échanges. Il devrait différencier l’enregistrement, la persistance des fichiers `.capture` et le fonctionnement du proxy : arrêter l’enregistrement ne coupe pas le transfert réseau.

Une lecture incrémentale avec curseur, filtres et délai maximal serait une première solution simple. Des notifications pourraient ensuite être proposées selon les capacités du client ; elles ne garantissent pas qu’un assistant reste actif en permanence.

## 4. Ce qui existe déjà et ce qu’il faudrait ajouter

Les points d’appui suivants sont présents dans le [contrat OpenAPI](webui/wwwroot/openapi.yaml) et les [handlers Web UI](webui/ui_server.go). Leur exposition via MCP reste à construire.

| Besoin | Point d’appui actuel | Travail supplémentaire pour MCP |
| --- | --- | --- |
| Lire l’état local | `/config`, `/api/proxy/state`, `/api/recording/state`, `/api/runtime/stats` | Vue synthétique et filtrage des informations sensibles |
| Examiner un échange | `/api/requests/{id}` et `/api/requests/{id}/body` | Résultats structurés, extraits bornés, masquage des secrets |
| Suivre les événements | SSE `/events` | Adaptation MCP, curseurs et gestion des pertes ou expirations |
| Rechercher dans le trafic en mémoire | Store et événements utilisés par la Web UI | Recherche serveur paginée ; pas de route générale de liste des requêtes dans le contrat actuel |
| Lire les captures sauvegardées | `/api/captures`, métadonnées et records par fichier | Sélection, filtrage et agrégation ; pagination des records déjà disponible |
| Envoyer une requête | `/api/composer/send` | Contrôle des actions autorisées et restitution claire du résultat |
| Préparer des collections | `/api/http-files` et opérations sur les fichiers `.http` | Conversion d’une capture et gestion des valeurs masquées |
| Lire ou modifier les réglages | `/api/settings/upstream`, `/api/settings/decrypt-https`, `/api/settings/body-capture` | Outils ciblés, permissions et indication du moment d’application |
| Examiner les certificats | `/certificates-infos` | Diagnostic lisible ; conserver les actions sur la confiance hors du premier périmètre |

Les modifications de réglages devraient réutiliser les mécanismes de persistance vers `config.yaml`. Pour l’upstream, le contrat actuel indique une prise d’effet au prochain démarrage : un résultat MCP devrait distinguer la configuration enregistrée de celle actuellement appliquée.

## 5. Une interface MCP possible

Les noms suivants sont illustratifs. Une première version pourrait proposer quelques outils ciblés :

| Outil proposé | Rôle |
| --- | --- |
| `get_status` | Résumer l’état du proxy, de l’enregistrement, de l’upstream et du MITM |
| `search_requests` | Rechercher par période, hôte, méthode et statut, avec limite et curseur |
| `get_request` | Lire les métadonnées et en-têtes filtrés d’un échange |
| `get_body_excerpt` | Lire un extrait borné du corps demandé, selon la politique de données |
| `list_captures` | Lister les captures disponibles |
| `read_capture_records` | Lire une page d’une capture |

Dans une deuxième étape, `set_recording`, `prepare_request`, `send_request` ou `update_upstream_settings` pourraient couvrir les actions explicitement autorisées.

Chaque résultat devrait préciser sa provenance, les identifiants utiles et ses limites : données masquées, corps absent, extrait tronqué, résultat paginé ou échange encore en cours. Les références aux fichiers devraient utiliser des noms validés dans le dossier de captures, sans accepter de chemin arbitraire.

L’adaptateur devrait distinguer une erreur de l’outil d’un échec réseau observé. Par exemple, `/api/composer/send` peut répondre HTTP 200 avec un champ `error` et un statut de destination nul : cela ne signifie pas que l’appel testé a réussi.

Des **ressources** pourraient donner accès à un état synthétique ou à une capture identifiée, par exemple `httpstacklens://status` et `httpstacklens://captures/{name}`. Ces URI seraient internes à l’intégration MCP. Des **prompts** pourraient guider des démarches récurrentes comme « diagnostiquer un échec de proxy » ou « comparer deux échanges ». Ils resteraient facultatifs ; les outils de lecture apporteraient déjà l’essentiel de la valeur.

## 6. Intégration dans le projet

Une approche simple serait d’ajouter un module Go dédié au MCP et de monter son handler sur le serveur HTTP local. Ce module appellerait les mêmes services que la Web UI pour lire les captures, consulter l’état ou envoyer une requête.

Cela éviterait de dupliquer la logique de forwarding, d’authentification Windows et de persistance. Un petit adaptateur externe consommant l’API HTTP pourrait servir de prototype, mais nécessiterait de compléter la recherche dans le trafic et la gestion des événements.

Les changements d’API Web UI nécessaires devraient être documentés dans `openapi.yaml` dans la même modification. Les schémas des outils MCP formeraient un contrat complémentaire, avec des entrées validées, des limites explicites et des erreurs compréhensibles.

## 7. Protection des données et contrôle des actions

Le contenu observé peut inclure des identifiants ou des données métier. Un serveur local n’implique pas que les données restent sur la machine : le client de l’assistant peut transmettre les résultats à un modèle distant.

Le périmètre proposé devrait donc prévoir :

- Un accès en lecture seule par défaut, avec les corps exclus des listes et consultables séparément.
- Un masquage côté serveur avant transmission : `Authorization`, `Proxy-Authorization`, cookies, paramètres sensibles des URL et champs sensibles des corps. Le masquage des seuls en-têtes serait insuffisant.
- Des limites de taille, de pagination et de durée pour éviter les réponses massives et préserver la réactivité du proxy.
- Une autorisation distincte pour les envois réseau, les écritures de fichiers, les changements de réglages et les suppressions. Les annotations MCP des outils ne remplacent pas les contrôles côté serveur.
- Le maintien du MITM comme choix explicite. L’installation ou la suppression de certificats et la modification des accès réseau seraient exclues de la première version.
- Le traitement des en-têtes et corps capturés comme des données non fiables : un texte reçu d’un site ne doit jamais devenir une instruction autorisant l’assistant à agir.

Pour le point d’entrée MCP, prévoir une écoute loopback, une validation de l’en-tête `Origin` lorsqu’il est présent et une authentification adaptée aux clients retenus. Ces protections sont décrites dans la [spécification Streamable HTTP](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports). L’authentification de l’accès MCP serait distincte de l’authentification Windows vers le proxy d’entreprise.

## 8. Première version recommandée

Le meilleur premier objectif serait : **permettre à un assistant d’expliquer une session de trafic sans modifier le comportement du proxy**.

1. Exposer l’état et les captures sauvegardées en lecture seule.
2. Ajouter une recherche paginée dans les échanges en mémoire et une lecture détaillée avec masquage.
3. Vérifier le parcours avec un client MCP HTTP : reproduire un échec, trouver les échanges et obtenir une explication fondée sur leurs résultats.
4. Ajouter ensuite la préparation de requêtes et, séparément, leur envoi autorisé via le Composer.

Les vérifications devraient couvrir notamment les données masquées, les limites de lecture, les captures expirées ou effacées, les corps indisponibles, le refus des actions non autorisées et l’absence d’impact sur le forwarding lorsque le client MCP est lent ou déconnecté.

La valeur principale serait de relier une question de développeur à des observations réseau concrètes : retrouver rapidement les échanges utiles, expliquer ce qu’ils montrent et préparer une vérification reproductible.
