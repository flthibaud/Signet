# Notice

Signet — un lecteur read-it-later self-hosted avec support natif RSS/Atom.

Copyright (C) 2026 Florian Thibaud

Ce programme est un logiciel libre : vous pouvez le redistribuer et/ou le modifier
selon les termes de la GNU Affero General Public License telle que publiée par la
Free Software Foundation, soit la version 3 de la licence, soit (à votre choix)
toute version ultérieure.

Ce programme est distribué dans l'espoir qu'il sera utile, mais **SANS AUCUNE
GARANTIE** ; sans même la garantie implicite de COMMERCIALISATION ou d'ADÉQUATION
À UN USAGE PARTICULIER. Voir la GNU Affero General Public License pour plus de
détails. Une copie complète est disponible dans le fichier [LICENSE](LICENSE) et
sur <https://www.gnu.org/licenses/agpl-3.0.html>.

## Marques

Le nom **Signet** et son logo ne sont **pas** couverts par la licence AGPL-3.0.
La licence vous accorde des droits sur le code source, pas sur l'identité du
projet. Un fork redistribué publiquement, ou une instance hébergée proposée
comme service, doit être publié sous un nom distinct afin d'éviter toute
confusion sur son origine.

## Inspiration

Signet est inspiré d'[Omnivore](https://github.com/omnivore-app/omnivore)
(AGPL-3.0), arrêté fin 2024 après le rachat de l'équipe par ElevenLabs.

**Aucun code d'Omnivore n'a été repris.** L'architecture, le schéma de base de
données et l'intégralité de l'implémentation sont originaux : Signet est un
binaire Go unique servant une SPA React embarquée, là où Omnivore reposait sur un
ensemble de services TypeScript. La parenté se limite à l'idée produit et à
certains choix d'ergonomie.

## Dépendances tierces

Signet est lié à des bibliothèques tierces, toutes distribuées sous licence
permissive (MIT, BSD ou Apache-2.0). Chacune reste soumise à sa propre licence ;
la liste complète et les versions exactes se trouvent dans [go.mod](go.mod) et
[frontend/package.json](frontend/package.json).
