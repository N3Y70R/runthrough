<!-- SPDX-License-Identifier: GPL-3.0-or-later -->

# runthrough — Especificación funcional

> **Estado:** borrador v0.1 · Documento vivo.
> Las decisiones cerradas están fechadas; las abiertas están en §9.

---

## 1. Qué es

`runthrough` es una herramienta de línea de comandos que **levanta un ecosistema de
servicios en local, en contenedores, construido directamente desde worktrees de git**, para
probar el estado actual del sistema y el efecto de un cambio concreto sobre él.

Es **agnóstica del lenguaje de los servicios**: orquesta por igual microservicios en Go, un
BFF en Node, un front en Angular o React, o un monolito PHP. El runtime de cada servicio es
un dato del catálogo, no una rama de código dentro de la herramienta.

Es hermana de [grove](https://github.com/N3Y70R/grove): grove gestiona el *código* (un
worktree por rama sobre un repositorio `.bare` compartido), `runthrough` gestiona la
*ejecución* de ese código.

> **El nombre.** Un *run-through* es el ensayo en que todas las piezas de una obra se
> ejecutan seguidas y juntas, antes del estreno.

## 2. El problema

Los entornos locales de desarrollo se degradan. Cada quien levanta los servicios a mano, en
puertos distintos, contra bases de datos distintas, desde la rama que quedó activa. Cuando
algo se rompe, nadie puede afirmar si fue el cambio o el entorno.

Para poder decir con evidencia **qué cambió el comportamiento del sistema entre el commit A
y el commit B** hacen falta tres cosas que casi ningún `docker-compose.yml` artesanal tiene
a la vez:

1. **Reproducibilidad** — el mismo punto de partida de datos en cada corrida.
2. **Trazabilidad** — saber qué commit de qué rama corre en cada contenedor.
3. **Ergonomía** — que cambiar de rama, de subconjunto de servicios o de infraestructura no
   sea un ritual manual propenso a error.

`runthrough` existe para que el entorno local sea un instrumento de medición y no una
anécdota.

## 3. Principios de diseño

1. **Un dominio, dos frentes.** Toda la lógica vive en la capa de dominio; la CLI y el
   servidor MCP son frentes delgados sobre las mismas operaciones.
2. **Salida estructurada primero.** Cada operación devuelve JSON; la CLI le da formato
   humano. Un agente razona sobre datos, no parsea texto.
3. **Escape hatch permanente.** Cada driver deja artefactos legibles y ejecutables a mano:
   archivos de compose en el driver de Compose, manifiestos en el que venga después. El día
   que la herramienta falle o estorbe, el runtime sigue funcionando sin ella.
4. **Declarativo, no imperativo.** El catálogo es la fuente de verdad de servicios,
   puertos, dependencias e infraestructura. Los comandos lo leen, no lo sustituyen.
5. **Seguro por defecto.** Las operaciones destructivas se bloquean cuando la
   infraestructura apuntada es compartida. Ningún secreto en la salida, los logs o las
   imágenes.
6. **No invadir los repos de servicio.** Nada que la herramienta necesite se versiona en el
   repositorio de cada servicio.

## 4. Decisiones cerradas

| # | Decisión | Fecha |
|---|---|---|
| D-1 | **Alcance multi-ecosistema** con catálogo modular: un archivo de servicios por dominio, compuestos con `include:` de Compose | 2026-09-09 |
| D-2 | **El worktree se resuelve con variables en el compose** (`context: ../../<repo>/${SVC_WT:-main}`), no generando el compose ni con symlinks | 2026-09-09 |
| D-3 | **CLI en Go** con servidor **MCP** en el mismo binario | 2026-09-09 |
| D-4 | **Nombre `runthrough`**, verificado libre en npm, PyPI y crates.io (ver Anexo B) | 2026-09-09 |
| D-5 | **Genérica por diseño**: la herramienta no conoce ninguna organización. Los catálogos son datos externos | 2026-09-09 |
| D-6 | **Agnóstica de lenguaje**: el runtime de cada servicio se declara en el catálogo | 2026-09-09 |
| D-7 | **Licencia GPL-3.0-or-later**, gobernanza por DCO sin cesión de copyright | 2026-09-09 |
| D-8 | **El runtime de contenedores es un driver**: el dominio no conoce Compose. Docker/Compose es la primera implementación, Podman la segunda; Kubernetes queda fuera de v1 | 2026-09-09 |
| D-9 | **El catálogo es un repositorio propio con forma de directorio**, localizado por una cadena de resolución explícita (ver §9) | 2026-09-09 |

## 5. Catálogo de funcionalidades

Prioridad: **M** = imprescindible para v1 · **S** = deseable en v1.x · **C** = futuro.

### Bloque 1 — Selección de código

| ID | Funcionalidad | Prio |
|---|---|---|
| G-01 | Contexto de build declarado como repo + worktree parametrizable, nunca ruta fija | M |
| G-02 | Perfiles de conjunto: un archivo define qué worktree usa cada servicio | M |
| G-03 | Soporte de repos sin worktrees (clon plano) | S |
| G-04 | Preflight: el worktree existe, es la rama esperada, árbol limpio, `.env` presente | M |
| G-05 | Trazabilidad: imagen etiquetada con repo, rama y commit; `status` los muestra | M |

> G-05 es lo que hace que un efecto observado sea atribuible a un cambio concreto.

### Bloque 2 — Exposición: gateway y sondas

| ID | Funcionalidad | Prio |
|---|---|---|
| N-01 | Gateway único con ruteo por prefijo, replicando el ruteo del entorno real | M |
| N-02 | Puerto de host por servicio, simultáneo al gateway, para sondear un servicio aislado | M |
| N-03 | Mapa de puertos sin colisiones y rango reservado por stack, para correr dos ecosistemas a la vez | M |
| N-04 | Perfiles para levantar subconjuntos (un flujo concreto, solo el backend, front + gateway) | S |
| N-05 | Modo "solo gateway", sin publicar puertos, para probar como lo ve un cliente real | C |
| N-06 | **Servicio en modo host**: el servicio que estás editando corre fuera del contenedor y el resto del stack lo alcanza igual; el gateway rutea sin cambios | M |
| N-07 | El gateway sirve también el front (SPA compilada o dev server) y sus assets | S |

### Bloque 3 — Infraestructura conmutable

| ID | Funcionalidad | Prio |
|---|---|---|
| I-01 | Tres modos: `local` (contenedores), `host` (la máquina del dev), `shared` (infraestructura remota compartida) | M |
| I-02 | Conmutación por perfil de archivo, sin editar servicio por servicio | M |
| I-03 | Granularidad por servicio: uno contra infraestructura compartida, el resto local | S |
| I-04 | Salvaguardas en modo `shared`: aviso visible y bloqueo de migraciones, seeds y reset | M |
| I-05 | Catálogo de infraestructura local: bases de datos, cache, broker de mensajes, almacenamiento S3 | M |

### Bloque 4 — Datos reproducibles

| ID | Funcionalidad | Prio |
|---|---|---|
| D-01 | Migraciones como job run-once con `service_completed_successfully` | M |
| D-02 | Snapshot y restore para volver al estado base entre pruebas | M |
| D-03 | Seeds y fixtures idempotentes por escenario | S |
| D-04 | Reset total con un comando | M |
| D-05 | Importar y anonimizar un volcado de un entorno superior como punto de partida | C |

> D-02 es lo que hace comparables dos corridas: sin punto de partida restaurable, un
> "antes y después" no significa nada.

### Bloque 5 — Build

| ID | Funcionalidad | Prio |
|---|---|---|
| B-01 | Autenticación de build por agente SSH, multi-origen (varios hosts y llaves) | M |
| B-02 | Dockerfile del repo cuando sirve; recetas parametrizadas cuando el del repo está atado al CI | S |
| B-03 | Caché de dependencias compartida entre builds | M |
| B-04 | Rebuild de un solo servicio | M |
| B-05 | Modo dev con código montado y recarga en caliente | M |
| B-06 | Recetas de build por runtime (Go, Node, PHP-FPM, SPA estática) declaradas en el catálogo | M |
| B-07 | Caché por runtime: módulos de Go, store de yarn/npm/pnpm, composer | M |
| B-08 | Auth de registries privados por `--mount=type=secret`, nunca horneada en la imagen | S |

### Bloque 6 — Salud y orden de arranque

| ID | Funcionalidad | Prio |
|---|---|---|
| H-01 | Healthcheck obligatorio por servicio | M |
| H-02 | `depends_on` con `condition` (healthy / completed_successfully) | M |
| H-03 | Smokes end-to-end por flujo | S |
| H-04 | `status` legible: perfil de infraestructura, rama y commit por servicio, salud, puertos | M |
| H-05 | Healthcheck configurable por tipo: HTTP, TCP o comando | M |

### Bloque 7 — Configuración y secretos

| ID | Funcionalidad | Prio |
|---|---|---|
| S-01 | `.env` del stack, `.env.example` versionado e interpolación de variables | M |
| S-02 | Los archivos locales de cada repo (`.env`, Dockerfiles de desarrollo) viven fuera del árbol versionado y se enlazan donde la herramienta los busca | M |
| S-03 | Regla `environment` > `env_file`, con overrides mínimos y comentados | M |
| S-04 | `.gitignore` / `.dockerignore` de secretos y hook pre-commit de detección | M |
| S-05 | Secretos compartidos coherentes entre servicios que deben coincidir | M |

### Bloque 8 — Hardening

| ID | Funcionalidad | Prio |
|---|---|---|
| Z-01 | `no-new-privileges`, usuario no-root, imágenes pinneadas | S |
| Z-02 | Cero credenciales horneadas en imágenes | S |
| Z-03 | CORS y cabeceras del gateway acotados a desarrollo | C |

### Bloque 9 — Experiencia de uso

| ID | Funcionalidad | Prio |
|---|---|---|
| X-01 | Comandos de operación: `up`, `down`, `status`, `probe`, `snapshot`, `restore`, `reset`, `logs` | M |
| X-02 | `doctor` de preflight con mensajes accionables | M |
| X-03 | Documentación numerada y checklist de verificación | S |
| X-04 | Integración opcional con interfaces de gestión de contenedores | C |

### Bloque 10 — La herramienta

| ID | Funcionalidad | Prio |
|---|---|---|
| T-01 | CLI en Go con subcomandos, salida humana y `--json` | M |
| T-02 | Servidor MCP en el mismo binario, sobre la misma capa de dominio | M |
| T-03 | Separación lectura / mutación; destructivas con confirmación y bloqueadas en `shared` | M |
| T-04 | Toda operación devuelve JSON estructurado | M |
| T-05 | Catálogo declarativo como única fuente de verdad | M |
| T-06 | Escape hatch: el compose sigue siendo ejecutable a mano | M |
| T-07 | Configuración de usuario en `~/.config/runthrough/` | M |
| T-08 | `doctor` valida versiones del runtime y coherencia del catálogo | S |
| T-09 | Cadena de resolución del catálogo, y `config show` que dice cuál se resolvió y por qué vía | M |

### Bloque 11 — Runtime intercambiable

| ID | Funcionalidad | Prio |
|---|---|---|
| R-01 | Puerto `Runner` (`up`, `down`, `rebuild`, `logs`, `status`, `exec`) con drivers intercambiables | M |
| R-02 | Vocabulario del catálogo **neutral**: `needs`, `expose`, `host_alias` — nunca términos propios de un runtime | M |
| R-03 | Matriz de capacidades por driver (build por SSH, condiciones de dependencia, bind mounts, red del host); `doctor` la reporta y `up` falla temprano en vez de a medias | M |
| R-04 | Driver de Podman | S |
| R-05 | La exposición se declara como **intención** (`gateway`, `direct`), y el driver la traduce a puertos publicados, port-forward o Ingress | M |
| R-06 | Kubernetes fuera de v1: cuando llegue será un driver que genere manifiestos o delegue en una herramienta de dev loop existente | C |

> R-02 y R-05 no cuestan nada hoy y son lo único que hace posible un segundo runtime
> mañana. R-03 es lo que evita la peor forma de fallo: un stack que arranca a medias porque
> el driver no soporta algo que el catálogo daba por hecho.

## 6. Arquitectura

Una capa de dominio, dos frentes. Ningún comando contiene lógica de negocio.

```
cmd/runthrough/            CLI + subcomando `mcp`
internal/catalog/          modelo declarativo: ecosistemas, servicios, perfiles
internal/worktree/         descubre repos y worktrees; resuelve rama y commit
internal/runner/           puerto Runner: up, down, rebuild, logs, status, exec
  compose/                 driver Docker Compose: resuelve variables e invoca docker compose
  podman/                  driver Podman: mismo modelo, semántica propia
internal/infra/            perfiles local/host/shared, resolución por servicio, salvaguardas
internal/data/             snapshot, restore, seed, reset
internal/probe/            healths, smokes, resolución interna de nombres
internal/doctor/           preflight
internal/report/           render humano y JSON de cualquier resultado
mcp/                       servidor MCP (stdio) sobre las mismas operaciones
compose/                   archivos de compose versionados, por ecosistema
```

- `internal/worktree` **lee el layout del disco**, no depende del binario de grove: la
  herramienta funciona igual con repos gestionados por worktrees o con clones planos.
- `internal/runner` es un **puerto**, no una implementación: el dominio habla de servicios,
  dependencias y exposición, nunca de Compose. Cada driver traduce a su runtime y declara
  qué capacidades soporta; el escape hatch se respeta dentro de cada driver.
- `internal/report` existe para que la salida JSON no sea un añadido tardío: toda operación
  produce una estructura y el render humano es una vista de ella.

## 7. Superficie de la CLI (borrador)

```
runthrough doctor
runthrough up      [--eco <nombre>] [--profile <perfil>]
                   [--infra local|host|shared] [--set <svc>=<rama>]
runthrough down    [--keep-data]
runthrough status                      # servicio · rama · commit · imagen · salud · puerto · infra
runthrough probe   <svc> | --all
runthrough rebuild <svc>
runthrough logs    <svc> [-f]
runthrough snapshot create|list|restore <nombre>
runthrough reset   [--hard]
runthrough worktree list|set <svc> <rama>
runthrough config  show|validate
```

Todos aceptan `--json`. Alias corto propuesto para el binario: `rt`.

## 8. Superficie MCP

**Lectura** — `rt_status`, `rt_probe`, `rt_catalog`, `rt_doctor`, `rt_logs`,
`rt_worktree_list`

**Mutación** — `rt_up`, `rt_down`, `rt_rebuild`, `rt_worktree_set`, `rt_snapshot`,
`rt_restore`, `rt_reset`

Reglas del frente MCP:

- Las operaciones de mutación declaran su efecto y exigen confirmación explícita.
- `rt_reset`, `rt_restore` y las migraciones están **bloqueadas** cuando el perfil de
  infraestructura resuelto es `shared`, sin importar quién las invoque.
- Ninguna respuesta incluye valores de secretos: solo si están presentes o ausentes.

## 9. El catálogo

### Forma y ubicación

El catálogo es un **repositorio propio**, versionado y revisable, independiente tanto del
código de la herramienta (D-5) como de los repos de servicio. Su forma es un directorio:

```
mi-catalogo/
├── runthrough.yaml     manifiesto raíz: ecosistemas, perfiles de infra, includes
├── compose/            un archivo por ecosistema
│   ├── backend.yml
│   └── data.yml
├── profiles/           perfiles de conjunto: qué worktree usa cada servicio
│   └── release.yaml
└── gateway/            configuración del gateway por ecosistema
```

Las rutas relativas del manifiesto se resuelven **contra el manifiesto**, nunca contra el
directorio de trabajo. `workspace:` admite ruta absoluta o `~`, así que el catálogo puede
vivir fuera del workspace donde están los repos de servicio.

Que sea un repositorio no es burocracia: hace que añadir un servicio al ecosistema pase por
revisión, quede en la historia y llegue igual a toda la gente del equipo.

### Cadena de resolución

Gana el primero que exista:

1. `--catalog <ruta>`
2. `RUNTHROUGH_CATALOG`
3. un `runthrough.yaml` buscando hacia arriba desde el directorio actual
4. el catálogo por defecto registrado en `~/.config/runthrough/config.yaml`

`runthrough config show` imprime cuál se resolvió **y por cuál de las cuatro vías**, porque
"¿qué catálogo estás usando?" es la primera pregunta cuando algo no cuadra.

Ningún secreto vive en el catálogo: los valores sensibles se referencian por nombre de
variable y residen en los `.env` (S-01).

### Manifiesto (ilustrativo)

```yaml
version: 1

ecosystems:
  backend:
    workspace: ~/work
    git_host: github
    ssh_key: ~/.ssh/id_ed25519_personal
    default_worktree: main
    compose: compose/backend.yml
    port_range: [8080, 8099]
    gateway:
      port: 8090
      routes:
        /orders: orders-api
        /users: users-api
        /: web-front
    services:
      orders-api:
        repo: orders-api
        runtime: go
        build: { recipe: go-ssh, binary: cmd/main.go }
        port: { internal: 8080, host: 8081 }
        health: { type: http, path: /health }
        needs: [users-api, postgres]

      bff:
        repo: bff
        runtime: node
        build: { recipe: node-yarn, node: 22 }
        port: { internal: 3000, host: 3000 }
        health: { type: http, path: /health }
        dev: { mount: true, command: "yarn start:dev" }
        needs: [orders-api, redis]

      web-front:
        repo: web-front
        runtime: angular
        build: { recipe: spa, node: 22, dist: dist/browser }
        port: { internal: 80, host: 4200 }
        health: { type: tcp }
        dev: { mount: true, command: "yarn start", port: 4200 }
        gateway_root: true

infra:
  profiles:
    local:  { postgres: container, redis: container, rabbitmq: container, s3: container }
    host:   { postgres: host, redis: host }
    shared: { postgres: remote, redis: remote, guard: read-only }
```

Cada servicio declara **qué necesita**, no **cómo se cablea**. El cableado —host, puerto,
URL interna— lo resuelve la herramienta según el perfil activo.

## 10. Decisiones abiertas

| # | Pregunta |
|---|---|
| A-1 | ¿Releases con binarios por plataforma (goreleaser) o solo `go install`? |
| A-3 | ¿El snapshot es por ecosistema o global? ¿Se versiona junto al catálogo o queda fuera de git? |
| A-4 | ¿La herramienta gestiona los archivos locales de cada repo o solo verifica que existan? |
| A-5 | ¿Qué versión mínima de cada runtime se soporta? |
| A-6 | Kubernetes: ¿el driver generaría manifiestos propios o delegaría en una herramienta de dev loop existente (Tilt, Skaffold, DevSpace)? |

## 11. Roadmap

**Fase 0 — Esqueleto.** Módulo Go, catálogo, `config show|validate`, `doctor`. Sin levantar
nada todavía: que la herramienta sepa leer el mundo.

**Fase 1 — Levantar.** `up` / `down` / `logs` / `rebuild` sobre un ecosistema, con worktrees
parametrizados, infraestructura `local` y `host`, healthchecks y orden de arranque.

**Fase 2 — Observar.** `status` con rama y commit por servicio, `probe`, gateway. Que el
stack sea un instrumento de medición.

**Fase 3 — Reproducir.** `snapshot` / `restore` / `reset`, migraciones como job,
salvaguardas de `shared`.

**Fase 4 — Abrir a los agentes.** Servidor MCP, salida JSON completa.

**Fase 5 — Multi-ecosistema.** Segundo catálogo, rangos de puertos disjuntos, perfiles por
flujo.

---

## Anexo A — Influencias

El diseño sale de la experiencia con dos orquestadores locales previos, cada uno bueno en
una mitad del problema:

- Uno con **toda la infraestructura contenerizada**, esquema creado desde cero por un job de
  migraciones, healthchecks y `depends_on` condicionado, y rutas conscientes de worktrees.
  Reproducible, pero atado a un ecosistema y a una rama fija.
- Otro con un **gateway único** que replicaba el ruteo del entorno real, disciplina de
  secretos por `.env` + plantilla versionada + detección en pre-commit, y hardening de
  contenedores. Pero con la infraestructura fuera del stack, sin reproducibilidad y sin
  healthchecks.

`runthrough` toma la reproducibilidad y el orden de arranque del primero, y el gateway, la
disciplina de secretos y el hardening del segundo, con dos añadidos propios: trazabilidad
por commit e infraestructura conmutable.

## Anexo B — El nombre

Criterios: herramienta libre, sin usurpar proyectos existentes, especialmente en devtools y
contenedores.

**Descartados con evidencia** (ocupados en el mismo espacio): `arbor` (CLI para gestionar
git worktrees), `coppice` (app de escritorio para worktrees y agentes), `copse` (workspace
agéntico), `canopy` (saturado, incluye un TUI para orquestar agentes), `trellis`
(orquestación de entornos), `shipyard` (entornos efímeros), `orchard` (orquestador de VMs),
`skaffold`, `harbor`, `gardener`, `biome`, `glade`, `sprig`, `depot`, `greenhouse`,
`propagator` (nombre propio en la especificación de OpenTelemetry), `cloche` (CLI activa),
`testbed` (API pública de Angular), `rehearse` (crate del mismo espacio conceptual).

**Elegido:** `runthrough` — libre en npm, PyPI y crates.io, sin software homónimo conocido,
y describe la función sin metáfora que traducir.
