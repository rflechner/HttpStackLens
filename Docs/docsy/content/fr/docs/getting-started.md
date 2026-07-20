---
title: Démarrage
linkTitle: Démarrage
weight: 20
description: >
  Construisez le binaire, lancez le proxy et ouvrez l'interface web en quelques minutes.
---

## Prérequis

| Outil                              | Pourquoi                                  |
|------------------------------------|-------------------------------------------|
| [Go 1.26.1+](https://go.dev/dl/)   | Compile le proxy et l'interface web WASM. |
| [Node.js](https://nodejs.org/)     | Construit le CSS Tailwind de l'interface. |

## Construction

Un outil de build en Go dans `build-tools/` exécute tout le pipeline — npm install,
compilation WASM, génération CSS et binaire natif. C'est la voie recommandée.

```sh
# Depuis la racine du projet — tout construire
go run ./build-tools/main.go

# Ou des cibles individuelles
go run ./build-tools/main.go webui   # Interface web seule (WASM + CSS)
go run ./build-tools/main.go app     # Binaire natif seul
go run ./build-tools/main.go --help  # Usage
```

L'outil de build détecte automatiquement votre plateforme et produit
`httpStackLens.exe` sous Windows ou `httpStackLens` sous macOS/Linux.

## Lancement

1. **Démarrer HttpStackLens.** Depuis la racine du projet :

   ```sh
   go run .
   ```

   Au premier lancement, un `config.yaml` est généré avec des valeurs par défaut
   sensées.

2. **Ouvrir l'interface web.** Rendez-vous sur <http://localhost:9000> pour observer
   le trafic en direct et piloter le proxy.

3. **Faire passer du trafic par le proxy.** Pointez n'importe quel client vers
   `localhost:3128` :

   ```sh
   curl -x http://localhost:3128 http://example.com
   ```

{{% alert title="🔑 Ports par défaut" color="info" %}}
Proxy → `3128`, interface web → `9000`. Les deux sont restreints au loopback par
défaut. Modifiez-les sous `proxy.port` / `webui.port` dans `config.yaml`.
{{% /alert %}}

## Pour aller plus loin

- [🏢 Proxy d'entreprise](../tutorial-upstream-proxy/) — se placer derrière un proxy
  d'entreprise authentifié sans assistant d'authentification séparé.
- [🔍 Déboguer le HTTPS & nettoyer](../tutorial-https-decrypt/) — déchiffrez le HTTPS,
  inspectez-le, puis effacez chaque certificat de votre OS.
