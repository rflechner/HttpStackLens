---
title: HttpStackLens
---

{{< blocks/cover title="Voyez à travers votre pile HTTP." image_anchor="top" height="med" color="white" >}}
<div class="mx-auto">
  <p class="lead mt-5">
    Un proxy de débogage HTTP/HTTPS local doté d'une interface web temps réel.
    Inspectez le trafic en direct, déchiffrez le HTTPS à la demande et transitez
    par un proxy d'entreprise authentifié — le tout depuis un unique binaire Go.
  </p>
  <a class="btn btn-lg btn-primary me-3 mb-4" href="docs/getting-started/">
    Commencer <i class="fas fa-arrow-alt-circle-right ms-2"></i>
  </a>
  <a class="btn btn-lg btn-outline-primary me-3 mb-4" href="docs/features/">
    Voir les fonctionnalités
  </a>
</div>
{{< /blocks/cover >}}

{{% blocks/lead color="primary" %}}
**Pour le développement local uniquement.** HttpStackLens est un outil de débogage
pour votre propre machine — Windows, macOS et Linux. Il n'est pas conçu pour la
production ni pour des réseaux partagés.
{{% /blocks/lead %}}

{{% blocks/section color="light" type="row" %}}

{{% blocks/feature icon="fa-random" title="Proxy direct" %}}
Écoute sur `localhost:3128`, tunnelise le HTTPS via `CONNECT` et relaie requêtes
et réponses de façon bidirectionnelle.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-unlock" title="Déchiffrement HTTPS" %}}
MITM local optionnel. Génère un certificat par domaine à partir d'un CA racine de
débogage pour lire les corps chiffrés.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-satellite-dish" title="Interface web temps réel" %}}
Une interface WASM + Tailwind diffuse le trafic via SSE : liste des requêtes,
timings, décodage des corps et aperçu d'images en ligne.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-building" title="Auth du proxy amont" %}}
Transite vers un proxy d'entreprise via NTLM, Kerberos et Negotiate sous Windows —
plus un mode de compatibilité 401/407.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-save" title="Capture & relecture" %}}
Enregistre les sessions dans un format binaire `.capture`, parcourez les captures
et interrogez le trafic via une API REST.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-broom" title="Nettoyage propre" %}}
Un clic retire le CA de débogage et chaque certificat qu'il a signé de votre
magasin de confiance — aucune trace laissée.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section color="dark" type="row" %}}

{{% blocks/feature icon="fa-building" title="S'authentifier derrière un proxy d'entreprise" url="docs/tutorial-upstream-proxy/" %}}
Pointez vos outils vers HttpStackLens et laissez-le gérer le proxy amont
authentifié — un simple proxy local sur votre poste de dev, sans assistant
d'authentification séparé.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-search" title="Déboguer le HTTPS, puis nettoyer" url="docs/tutorial-https-decrypt/" %}}
Activez le déchiffrement, inspectez le trafic chiffré, puis retirez chaque
certificat pour garder votre OS intact.
{{% /blocks/feature %}}

{{% /blocks/section %}}
