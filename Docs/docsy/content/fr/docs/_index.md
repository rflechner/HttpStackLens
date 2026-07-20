---
title: Documentation
linkTitle: Documentation
weight: 20
---

Tout ce qu'il faut pour construire, lancer et utiliser **HttpStackLens** — un proxy
de débogage HTTP/HTTPS local doté d'une interface web temps réel.

## Démarrage rapide

```sh
# Tout construire (WASM + CSS + binaire natif)
go run ./build-tools/main.go

# Lancer le proxy + l'interface web
go run .

# Envoyer une requête à travers lui
curl -x http://localhost:3128 http://example.com
```

Le proxy écoute sur `localhost:3128` et l'interface web sur `localhost:9000`. Voir
[Démarrage](getting-started/) pour les prérequis et la construction.
