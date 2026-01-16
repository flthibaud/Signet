
# Omnivore Go API

|Verbe|Chemin|Handler|Description|
|-----|------|-------|-----------|
|GET|/v1/healthcheck|HealthcheckHandler|Vérifier que l'API tourne.|
|POST|/v1/links|CreateLinkHandler|Ajoute une URL (Parse + Save).|
|GET|/v1/links|ListLinksHandler|La liste paginée (Ta question).|
|GET|/v1/links/:id|GetLinkHandler|Détail d'un article (Contenu HTML).|
|PATCH|/v1/links/:id|UpdateLinkHandler|Archiver, Marquer lu, Favori.|
