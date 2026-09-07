# Environnement OAuth local

[English](README.md) | Français

Keycloak, une API Go et une petite application navigateur pour développer le
support OAuth du Composer. Aucun changement du Composer n'est nécessaire pour
envoyer manuellement les requêtes de `requests.http`.

## Démarrage

Prérequis : Docker avec Compose v2 et moteur démarré. Depuis ce dossier :

```sh
docker compose up --build --wait --wait-timeout 180
```

Depuis la racine du dépôt, ajouter `-f tests/integration/oauth/compose.yaml`
aux commandes Compose. Le premier lancement télécharge Keycloak et l'image Go.
La santé de l'API vérifie que le realm et le client d'introspection sont prêts.

| Service | Adresse / identifiants de démonstration |
| --- | --- |
| Application et API | http://localhost:18081 |
| Console Keycloak | http://localhost:18080/admin — `admin` / `admin-demo-only` |
| Utilisateur | `alice` / `alice-demo-password` |
| Realm | `httpstacklens` |
| Client machine | `composer-service` / `composer-demo-secret` |
| Client navigateur public | `demo-app`, sans secret, PKCE S256 obligatoire |
| Client d'introspection de l'API | `demo-api` / `demo-api-secret` |

Tout est jetable : aucune base externe ni volume de données persistant.
Les mots de passe sont des fixtures publiques, réservées à ce test local.
Les ports sont publiés uniquement sur la boucle locale. Le banc utilise HTTP
local ; il ne couvre pas encore les certificats HTTPS ni le MITM.

```sh
docker compose down
# Recrée également le realm après modification du JSON :
docker compose up --build --force-recreate --wait --wait-timeout 180
```

Keycloak ignore l'import d'un realm déjà présent. Pour repartir explicitement
de zéro, utiliser `down`, puis `up`. Voir la
[documentation d'import Keycloak](https://www.keycloak.org/server/importExport).

## Essayer l'application

Ouvrir **http://localhost:18081** (utiliser `localhost`, pas `127.0.0.1`, pour
respecter la redirect URI enregistrée). Choisir une connexion avec ou sans
`demo:read`, puis se connecter avec Alice. L'application effectue un échange
Authorization Code avec PKCE S256 et vérification du `state`.

- `/public` : `200`, sans authentification.
- `/protected` : `200` avec un access token valide, sinon `401`.
- `/scoped` : `200` avec `demo:read`, `403` avec un token valide sans ce scope.
- « Refresh token » (renouveler le token) : utilise le refresh token du parcours navigateur.
- Les access tokens expirent après 120 secondes. Attendre puis appeler l'API
  permet de constater un `401`, avant de renouveler le token.

Les tokens restent en mémoire JavaScript et ne sont pas affichés. Seuls le
verifier PKCE et le state transitent temporairement par `sessionStorage` pour
survivre à la redirection. Recharger la page efface les tokens, mais ne ferme
pas la session SSO Keycloak. Les flux implicite et mot de passe sont désactivés.

## Essayer le Composer

Copier `oauth-login.http` ou `oauth-service.http` dans le dossier configuré par
`http_files.folder`, puis envoyer une requête dans le Composer.

- `oauth-login.http` ouvre Keycloak avec PKCE : Alice / `alice-demo-password`.
- `oauth-service.http` obtient un token client credentials sans fenêtre.
- Le token est réutilisé pour ce fichier, en mémoire uniquement. **Reset scope**
  l'efface ; le prochain envoi réévalue les variables. Une modification des
  déclarations ou l'expiration connue du token réinitialise aussi le scope.
- Le reset ne ferme pas la session SSO Keycloak. L'exemple utilise `prompt: 'login'`
  pour redemander la connexion. Fermer la popup annule l'envoi en attente.
- Retirer `scope: 'demo:read'` permet de vérifier le `403` sur `/scoped`.

`requests.http` reste disponible pour les tests manuels avec copie du token.
Les appels OAuth passent directement par le backend (proxy d'environnement Go),
pas par le proxy upstream configuré dans HttpStackLens. Les requêtes API passent
par le pipeline habituel et leurs headers peuvent apparaître dans la capture.

Endpoints OAuth :

```text
Issuer:        http://localhost:18080/realms/httpstacklens
Discovery:     http://localhost:18080/realms/httpstacklens/.well-known/openid-configuration
Authorization: http://localhost:18080/realms/httpstacklens/protocol/openid-connect/auth
Token:         http://localhost:18080/realms/httpstacklens/protocol/openid-connect/token
```

Le client public `composer-browser` accepte les callbacks
`http://localhost:9000/api/composer/oauth/callback` et
`http://127.0.0.1:9000/api/composer/oauth/callback`. Si le port de la Web UI change,
ajouter son URI exacte dans Keycloak. Recréer les conteneurs avec `down`, puis
`up --build --wait` pour importer ce nouveau client. `demo-app` garde son callback
`http://localhost:18081/`.

Avec un proxy upstream configuré dans HttpStackLens, inclure `localhost` et
`127.0.0.1` dans `no_proxy` pour joindre ces fixtures locales.

## Validation

Test du handler OAuth contre la fixture démarrée, depuis la racine du dépôt :

```sh
HSL_TEST_KEYCLOAK=1 go test ./webui -run TestComposerOAuthKeycloak -v
```

Ce test vérifie client credentials et les réponses `200`/`403` de l'API, sans
lancer de Web UI. La popup se vérifie manuellement avec `oauth-login.http`.


Tests Go sans Docker, depuis ce dossier (module indépendant, sans dépendance) :

```sh
cd api
go test ./...
```

Les tests de ce module ne sont pas inclus dans `go test ./...` à la racine.

Utiliser les requêtes du Composer et l'application de démonstration décrites
ci-dessus pour vérifier manuellement les parcours OAuth avec Keycloak.

## Vérification des tokens et réseau Docker

L'API délègue la validation cryptographique et l'état du token à l'introspection
Keycloak, avec son client confidentiel. Elle exige ensuite `active=true`, le bon
issuer, l'audience `demo-api`, une expiration future et, pour `/scoped`, le scope
exact. Elle ne traite pas un simple JWT décodé comme une preuve d'authenticité.
Une indisponibilité de Keycloak produit `503` et n'autorise pas la requête.

L'issuer public reste `http://localhost:18080/realms/httpstacklens` grâce à
`KC_HOSTNAME`. L'API appelle l'introspection sur `http://keycloak:8080` depuis
Docker : cette adresse interne ne remplace jamais l'issuer attendu. Voir la
[documentation hostname Keycloak](https://www.keycloak.org/server/hostname).

Les deux clients appelants reçoivent explicitement l'audience `demo-api` via un
mapper. `demo:read` est un scope optionnel de démonstration, sans modèle de rôles
métier. Le client d'introspection et son secret sont codés en dur uniquement
dans cette API de test. Aucun token ni corps de requête n'est journalisé par elle.
