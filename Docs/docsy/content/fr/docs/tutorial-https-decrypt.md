---
title: "Tutoriel : déboguer le HTTPS & nettoyer les certificats"
linkTitle: Déchiffrement HTTPS
weight: 40
description: >
  Activez le déchiffrement HTTPS, inspectez le trafic chiffré, puis retirez chaque
  certificat de débogage pour garder votre magasin de confiance intact.
---

Déchiffrer le HTTPS implique de faire confiance à un CA racine généré localement. Ce
tutoriel montre comment activer le déchiffrement, lire le trafic chiffré en clair, et
— surtout — retirer ensuite chaque certificat pour ne pas laisser traîner un CA de
débogage dans le magasin de confiance de votre OS.

{{% alert title="⚠️ Vous installez un CA racine de confiance" color="warning" %}}
Le déchiffrement fonctionne en ajoutant un certificat racine de débogage au magasin
de confiance de votre OS et en signant un faux certificat pour chaque site visité.
C'est puissant pour déboguer et risqué si on l'oublie — quiconque volerait la clé du
CA pourrait usurper des sites auprès de votre machine. **Exécutez toujours le
nettoyage de la section 4 une fois terminé.** N'utilisez jamais cela sur une machine
partagée.
{{% /alert %}}

## 1. Activer le déchiffrement HTTPS

Le déchiffrement est désactivé par défaut. Vous pouvez l'activer de deux façons.

### Option A — depuis le fichier de config

```yaml
decrypt_https:
  enabled: true   # intercepte & déchiffre le HTTPS (génère + installe un certificat par domaine)
  cert_manager:
    ca_cert_file: "certificates/debug-https-ca.crt"
    ca_key_file: "certificates/debug-https-ca.key"
    domain_certs_folder: "certificates/domains"
  mime_types:
    - name: "application/json"
    - name: "text/*"
      max_size_kb: 10000
```

Les règles `mime_types` décident quels corps de réponse sont capturés et jusqu'à
quelle taille — pratique pour éviter de bufferiser de gros téléchargements binaires.

### Option B — basculer en direct depuis l'interface web

Ouvrez <http://localhost:9000> et actionnez l'interrupteur de déchiffrement. Aucun
redémarrage nécessaire — en coulisses, l'interface appelle
`POST /api/settings/decrypt-https`.

![Bascule du déchiffrement HTTPS depuis l'interface web.](/images/screenshots/decrypt-toggle.png)

## 2. Générer & faire confiance au CA de débogage

Au premier déchiffrement, HttpStackLens génère un CA racine de débogage et l'installe
dans votre magasin Windows / trousseau macOS, puis émet un certificat par domaine
pour chaque hôte visité. Depuis l'interface, vous pouvez générer, installer et
exporter le CA explicitement :

| Action   | Endpoint                          | Rôle                                                          |
|----------|-----------------------------------|--------------------------------------------------------------|
| Générer  | `/api/certificates/ca/generate`   | Crée la clé + le certificat du CA racine sur disque.         |
| Installer| `/api/certificates/ca/install`    | Ajoute le CA racine au magasin de confiance de l'OS.         |
| Exporter | `/api/certificates/ca/export`     | Télécharge le certificat du CA (ex. pour Firefox).           |

{{% alert title="🏷️ Comment le nettoyage reste sûr" color="info" %}}
Chaque CA créé par l'application intègre le marqueur `My Local CA for debugging
HTTPS` dans son sujet, et chaque certificat qu'il signe le porte dans son émetteur
(issuer). Le nettoyage se base sur cette chaîne exacte, il ne retire donc que les
certificats créés par HttpStackLens — jamais rien d'autre dans votre magasin de
confiance.
{{% /alert %}}

## 3. Inspecter le trafic déchiffré

Envoyez du trafic HTTPS à travers le proxy et observez-le se déchiffrer en direct :

```sh
curl -x http://localhost:3128 https://api.github.com/zen
```

Dans l'interface web, les requêtes déchiffrées sont signalées comme telles — vous
obtenez en-têtes et corps en clair, avec timings, décodage base64 des corps et aperçu
d'images en ligne. Les panneaux requête/réponse côte à côte vous laissent vous
concentrer sur l'un ou l'autre.

![Un échange HTTPS déchiffré, entièrement lisible dans l'interface.](/images/screenshots/decrypted-inspect.png)

## 4. Nettoyer — retirer chaque certificat

C'est la partie importante : **ne laissez pas le CA de débogage installé.** Une fois
le débogage terminé, lancez le nettoyage. Dans l'interface web, utilisez l'action de
nettoyage des certificats ; elle appelle `POST /api/certificates/cleanup`, qui, en
une seule fois :

- Désactive le déchiffrement HTTPS.
- Retire le CA racine de débogage — et chaque certificat par domaine qu'il a signé —
  du magasin de confiance de l'OS (identifiés par le marqueur).
- Supprime le dossier des certificats par domaine (`certificates/domains`).
- Supprime le certificat et la clé du CA racine du disque.

![Le récapitulatif de nettoyage : CA racines et certificats de domaine retirés.](/images/screenshots/cert-cleanup.png)

{{% alert title="🖥️ Note par plateforme" color="warning" %}}
Le nettoyage automatique du magasin de confiance de l'OS est implémenté par
plateforme. Là où il n'est pas disponible, le récapitulatif vous le signale et les
fichiers sur disque sont tout de même supprimés — vous retirez alors le CA racine à
la main du magasin de confiance de votre OS. Le sujet du CA contient `My Local CA for
debugging HTTPS`, il est donc facile à repérer.
{{% /alert %}}

## 5. Vérifier que votre OS est propre

1. **Vérifier le rapport de nettoyage.** L'interface indique combien de certificats
   racine et de domaine ont été retirés. Vérifiez que les compteurs sont cohérents et
   qu'il n'y a pas d'avertissement.

2. **Recontrôler le magasin de confiance.** Cherchez le CA de débogage dans le
   magasin de confiance de votre OS pour confirmer sa disparition :

   ```sh
   # macOS — ne devrait rien afficher
   security find-certificate -a -c "My Local CA for debugging HTTPS" \
     ~/Library/Keychains/login.keychain-db

   # Windows (PowerShell) — ne devrait rien afficher
   Get-ChildItem Cert:\CurrentUser\Root |
     Where-Object { $_.Subject -like "*My Local CA for debugging HTTPS*" }
   ```

3. **Confirmer que les fichiers ont disparu.**

   ```sh
   ls certificates/            # .crt / .key du CA retirés
   ls certificates/domains/    # dossier supprimé
   ```

{{% alert title="✅ OS propre" color="success" %}}
Le déchiffrement est désactivé, le CA de débogage et chaque certificat qu'il a signé
ont disparu du disque comme du magasin de confiance de l'OS. Votre machine est
revenue exactement à son état initial.
{{% /alert %}}
