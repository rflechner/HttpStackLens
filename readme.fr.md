# HttpStackLens

[English](readme.md) · **Français**

![HttpStackLens](images/splash-screen.png)

[![Release](https://github.com/rflechner/HttpStackLens/actions/workflows/release.yml/badge.svg)](https://github.com/rflechner/HttpStackLens/actions/workflows/release.yml)
[![Dernière version](https://img.shields.io/github/v/release/rflechner/HttpStackLens)](https://github.com/rflechner/HttpStackLens/releases)
[![Version de Go](https://img.shields.io/github/go-mod/go-version/rflechner/HttpStackLens)](go.mod)
[![Licence](https://img.shields.io/github/license/rflechner/HttpStackLens)](LICENSE)

> **Travail en cours** — ce projet est loin d'être terminé et évolue au fil du temps.

HttpStackLens est un proxy HTTP/HTTPS local conçu **pour le développement local uniquement**. Il permet d'inspecter et de visualiser le trafic HTTP qui transite entre un client et un serveur, comme un outil minimal de débogage réseau.

## Motivation

Ce projet est avant tout un **exercice d'apprentissage de Go**. L'objectif est de me familiariser avec le langage et d'explorer ses idiomes en les comparant à ce que je fais habituellement en **C#** et **F#** — gestion des erreurs, concurrence, structuration du code, etc.

## Ce qu'il fait (pour l'instant)

- Écoute les connexions entrantes sur un port local (`3128` par défaut)
- Gère les tunnels HTTPS via la méthode `CONNECT`
- Peut déchiffrer le trafic HTTPS par MITM local, sur activation explicite de `decrypt_https.enabled`
- Relaie requêtes et réponses dans les deux sens
- Application desktop Wails avec un inspecteur de trafic en Go/WASM

## Ce qu'il ne fait pas (encore)

- Il n'est pas destiné à la production ni aux réseaux partagés

## Récupérer l'application

Chaque version taguée publie des archives précompilées sur la
[page des releases](https://github.com/rflechner/HttpStackLens/releases), pour
Windows et macOS en `amd64` comme en `arm64`. Chaque archive contient le binaire,
un `config.yaml` d'exemple et les notes de version ; un fichier
`checksums_<version>.txt` posé à côté permet de vérifier ce que vous avez
téléchargé :

```sh
sha256sum -c checksums_v0.2.0-alpha1.txt
```

Sous Windows, en l'absence de `sha256sum` :

```powershell
Get-FileHash .\httpStackLens_v0.2.0-alpha1_windows_amd64.zip -Algorithm SHA256
```

**Cela dit, compiler vous-même reste la meilleure option.** Trois raisons :

1. **Les binaires ne sont pas signés.** Aucun certificat Authenticode n'y est
   attaché : Windows SmartScreen affichera donc un avertissement, et les
   antivirus peuvent carrément les bloquer. Ces détections sont des faux
   positifs, mais de l'extérieur rien ne permet de distinguer un faux positif
   d'une vraie détection.
2. **L'application ressemble à un malware, par construction.** Elle installe une
   autorité de certification racine dans votre magasin de confiance et
   intercepte le TLS — c'est tout l'intérêt d'un proxy de débogage, et c'est
   aussi exactement ce que fait l'outil d'un attaquant. Les scanners
   heuristiques ne peuvent pas faire la différence, et vous non plus à partir
   d'un simple binaire.
3. **Elle demande une confiance réelle.** L'exécuter, c'est donner à un
   programme la capacité de lire votre trafic HTTPS en clair. C'est beaucoup
   accorder à un binaire compilé sur la machine de quelqu'un d'autre. Compiler
   depuis les sources garantit que le code que vous avez audité est bien celui
   que vous exécutez.

La compilation tient en une seule commande et ne demande que Go et Node.js —
voir [Compilation](#compilation) ci-dessous.

## Prérequis

- [Go](https://go.dev/dl/) 1.26.1 ou supérieur
- [Node.js](https://nodejs.org/) (pour la compilation de l'interface web — Tailwind CSS)

## Compilation

Un outil de build en Go dans `build-tools/` prend en charge toute la chaîne (npm install, compilation WASM, génération du CSS, binaire natif). **C'est la manière recommandée de compiler le projet.**

Si vous préférez dérouler chaque étape à la main, voir [Étapes manuelles](#étapes-manuelles) plus bas.

### Avec l'outil de build

Depuis la racine du projet — **c'est tout ce dont vous avez besoin :**

```sh
go run .\build-tools\main.go
```

Cibles supplémentaires :

```sh
go run .\build-tools\main.go webui        # Interface web seule (WASM + CSS)
go run .\build-tools\main.go app          # Application Wails autonome → build/bin
go run .\build-tools\main.go --help       # Aide
```

Les builds de release et multi-architectures peuvent cibler une plateforme Wails explicite :

```sh
go run .\build-tools\main.go -platform windows/arm64 app
go run .\build-tools\main.go -platform darwin/amd64 app
```

L'outil de build fait également référence pour les tags de production Wails, les
ressources natives, la stratégie WebView2 sous Windows et les métadonnées de
version. N'utilisez `-skip-frontend` que si `webui/wwwroot` a déjà été compilé.

Ou via les scripts npm depuis `webui/` :

| Commande | Rôle |
|---|---|
| `npm run build` | Interface web + binaire natif |
| `npm run build:webui` | WASM + Tailwind CSS uniquement |
| `npm run build:app` | Application Wails autonome dans `build/bin/` |
| `npm run dev:css` | Tailwind CSS en mode watch (dev) |

La cible par défaut recompile l'interface web et produit une application desktop
packagée avec sa fenêtre native, son icône et ses métadonnées de plateforme :

```sh
go run .\build-tools\main.go
```

L'application autonome est écrite dans `build/bin/`. Sous Windows, le build
s'appuie sur le runtime WebView2 déjà présent sur la machine — il est livré avec
Windows 11 et les versions récentes de Windows 10, et une machine qui ne l'a pas
est renvoyée vers la page de téléchargement de Microsoft. Le bootstrapper n'est
délibérément *pas* embarqué : cela placerait un second exécutable à l'intérieur
du binaire, destiné à être écrit sur disque puis lancé, ce que les heuristiques
antivirus interprètent comme un malware. Le premier build peut télécharger la
version épinglée de la CLI Wails si elle n'est pas déjà installée. Lancer
`go run -tags=dev .` reste utile pendant le développement Go et ouvre également
la fenêtre desktop Wails.

### JetBrains GoLand

Wails exige des tags de build ; une configuration GoLand standard sans tags
ouvre une fenêtre d'erreur au lieu de l'application.

Créez une configuration **Go Build** via **Run → Edit Configurations…** avec :

| Champ | Valeur |
|---|---|
| Name | `HttpStackLens` |
| Run kind | `Package` |
| Package path | `httpStackLens` |
| Working directory | la racine du projet, par exemple `C:\dev\HttpStackLens` |
| Go tool arguments (Run) | `-tags=desktop,production` |
| Go tool arguments (Debug) | `-tags=dev` |
| Program arguments | vide, sauf si une option de proxy est nécessaire |

Utilisez les tags `dev` au lancement avec le débogueur. Dans ce mode, les
fichiers statiques de l'interface web sont lus depuis `webui/wwwroot`, si bien
que les modifications HTML et JavaScript sont visibles après un rechargement.
Recompilez le WASM et le CSS générés après avoir modifié le frontend Go/WASM ou
les sources Tailwind :

```powershell
go run .\build-tools\main.go webui
```

Arrêtez ensuite complètement l'instance précédente de l'application avant de la
relancer depuis GoLand. HttpStackLens utilise un verrou d'instance unique Wails :
une fenêtre déjà ouverte empêche le démarrage d'une seconde instance.

---

### Étapes manuelles

<details>
<summary>Cliquer pour déplier</summary>

#### 1. Installer les dépendances npm

```sh
cd webui
npm install
```

#### 2. Copier wasm_exec.js

```sh
# macOS / Linux
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" webui/wwwroot/js/wasm_exec.js

# Windows (PowerShell)
Copy-Item "$(go env GOROOT)\lib\wasm\wasm_exec.js" -Destination webui\wwwroot\js\wasm_exec.js
```

#### 3. Compiler Go vers WASM

```sh
# macOS / Linux
GOOS=js GOARCH=wasm go build -o webui/wwwroot/wasm/app.wasm ./webui/wasm

# Windows (PowerShell)
$env:GOOS = "js"; $env:GOARCH = "wasm"
go build -o webui\wwwroot\wasm\app.wasm .\webui\wasm
$env:GOOS = ""; $env:GOARCH = ""
```

#### 4. Compiler le CSS Tailwind

```sh
cd webui
npx tailwindcss -i ./src/input.css -o ./wwwroot/css/output.css --minify
```

#### 5. Compiler le binaire natif

```sh
# macOS / Linux
go build -tags=desktop,production -o httpStackLens .

# Windows
go build -tags=desktop,production -ldflags="-H windowsgui" -o httpStackLens.exe .
```

> Pas de `-s -w` ici : ce build manuel conserve donc ses symboles. Les builds de
> release passent par la CLI Wails, qui les retire d'elle-même sur tout build de
> production.

</details>

---

### Compilation croisée

Préférez l'outil de build, afin que la sortie multi-architecture reçoive les
mêmes tags de production Wails et les mêmes ressources natives qu'un build de
release local :

**Windows ARM64 :**

```powershell
go run .\build-tools\main.go -platform windows/arm64 app
```

**macOS Intel depuis un runner Apple Silicon :**

```sh
CGO_CFLAGS="-arch x86_64" CGO_LDFLAGS="-arch x86_64" \
  go run ./build-tools/main.go -platform darwin/amd64 app
```

Wails ne permet pas de compiler une application macOS depuis Windows. Le
workflow de release utilise donc des runners Windows et macOS natifs, et ne
franchit que l'architecture CPU lorsque c'est nécessaire.

### Fonctionnalités spécifiques à Windows

Deux options ne sont disponibles que sous Windows (compilées automatiquement lorsque la cible est `GOOS=windows`) :

- `--windows-auth-require-ntlm` — exiger une authentification NTLM/Negotiate des clients qui se connectent
- `--output-proxy-add-windows-auth` — injecter les identifiants Windows lors du relais vers un proxy amont

Elles reposent sur l'API SSPI de Windows (`secur32.dll`) et renvoient une erreur si elles sont utilisées sur une autre plateforme.

## Utilisation

```sh
go run -tags=dev .
```

La fenêtre Wails s'ouvre automatiquement, et le proxy écoute sur `localhost:3128`.
Vous pouvez le tester avec curl :

```sh
curl -x http://localhost:3128 http://example.com
```
