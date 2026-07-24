# Pharus

![Portada de Pharus](docs/portada.jpg)

[English version](README.en.md)

Pharus es un motor local de investigación profunda escrito en Go. Planifica una
investigación, consulta SearXNG, extrae evidencia de páginas web, indexa el corpus
en un almacén vectorial y genera un informe Markdown mediante un LLM.

Puede ejecutarse directamente desde la CLI o como un demonio que expone un
socket Unix, una API HTTP y un endpoint compatible con Model Context Protocol
(MCP). El flujo de investigación incluye perfiles de expertos inspirados en
Co-STORM, refinamiento de esquemas TAXMORPH, análisis de vacíos, síntesis por
secciones, validación de citas y un bucle de crítica configurable.

> **Estado del proyecto:** algunas integraciones son experimentales. Consulta
> [Limitaciones actuales](#limitaciones-actuales) antes de usar Pharus en
> producción.

> **Seguridad:** la configuración puede contener credenciales. No publiques
> `.env` ni `~/.pharus/config.yaml`.

## Funcionalidad

- Planificación de consultas y perfiles `quick`, `balanced` y `deep`.
- Búsqueda a través de una instancia SearXNG con salida JSON.
- Extracción HTML con límite de 5 MiB, bloqueo de direcciones privadas y
  loopback, validación de redirecciones y fijación de DNS.
- Proveedores LLM: MiniMax, OpenAI, Anthropic, Ollama y llama.cpp.
- Proveedores de embeddings: OMLX, MLX, Ollama y APIs externas compatibles con
  OpenAI.
- Almacenamiento vectorial persistente mediante `chromem-go`, con aislamiento
  de resultados por identificador de investigación.
- Integración opcional con LanceDB nativo mediante `lancedb/lancedb-go`.
- Checkpoints de planificación y síntesis para reanudar investigaciones.
- Informes con estados `success`, `degraded` o `failed`; Pharus no genera un
  informe cuando no obtiene evidencia suficiente.
- Demonio HTTP/UDS y servidor MCP con investigaciones síncronas y asíncronas.
- Evaluación RACE y suites de benchmark de muestra.

## Requisitos

- Go 1.25 o posterior.
- Una instancia SearXNG con el formato JSON habilitado.
- Un proveedor LLM configurado.
- Un servicio de embeddings configurado.
- macOS para administrar el demonio con `launchd`.
- Docker, opcional, para desplegar SearXNG y usar el backend de ejecución en
  contenedor.

El motor puede compilarse en otros sistemas soportados por Go, pero la
autoconfiguración, OMLX administrado y `launchd` están orientados a macOS; OMLX
administrado requiere macOS sobre Apple Silicon.

## Instalación rápida

```bash
git clone https://github.com/KyberixCo/Pharus.git
cd Pharus
go build -o bin/pharus ./cmd/pharus
./bin/pharus config
./bin/pharus doctor
```

El asistente `config` configura los proveedores, puede preparar servicios
locales y guarda la configuración en `~/.pharus/config.yaml` y en `.env`.

Como alternativa, `setup` crea los directorios de trabajo, guarda la
configuración e intenta desplegar una instancia administrada de SearXNG si
Docker está disponible:

```bash
./bin/pharus setup
./bin/pharus doctor --fix
```

`doctor --fix` sólo intenta reparar automáticamente el servicio OMLX cuando ese
es el proveedor de embeddings seleccionado.

## Uso de la CLI

Ejecutar una investigación en el proceso actual:

```bash
./bin/pharus research --direct --language es \
  "Avances recientes en computación cuántica"
```

Por defecto, el informe se guarda en el directorio actual con extensión `.md`.
También puedes elegir la ruta y mostrar el informe en la terminal:

```bash
./bin/pharus research --direct --output reporte.md --stdout \
  --language es "Computación cuántica"
```

Los flags globales `--language` y `--lang` aceptan `auto`, `en` o `es`. El flag
`--profile` acepta:

| Perfil | Uso previsto |
| --- | --- |
| `quick` | Menos consultas y secciones; sin iteraciones de crítica. |
| `balanced` | Configuración predeterminada. |
| `deep` | Más consultas, secciones, contexto e iteraciones de crítica. |

Ejemplos:

```bash
./bin/pharus research --direct --profile quick "Resumen ejecutivo"
./bin/pharus research --direct --profile balanced "Análisis técnico"
./bin/pharus research --direct --profile deep "Revisión exhaustiva"
```

Las ejecuciones directas reanudan checkpoints compatibles de forma
predeterminada. Usa `--no-resume` para ignorarlos o `--restart` para eliminar el
checkpoint del mismo tema, idioma y perfil:

```bash
./bin/pharus research --direct --no-resume "Tema"
./bin/pharus research --direct --restart "Tema"
```

Sin `--direct`, la CLI intenta usar el socket Unix del demonio y, si no puede
conectarse, ejecuta la investigación en el proceso actual.

### Demonio

En macOS:

```bash
./bin/pharus daemon start
./bin/pharus daemon status
./bin/pharus research "Impacto de la IA en la medicina"
./bin/pharus daemon stop
```

Para ejecutarlo en primer plano, también en otros sistemas:

```bash
./bin/pharus daemon run
```

De forma predeterminada escucha en:

- Socket Unix: `~/.pharus/pharus.sock`
- HTTP: `http://127.0.0.1:8765`
- MCP Streamable HTTP: `http://127.0.0.1:8765/mcp`
- Eventos SSE de tareas: `http://127.0.0.1:8765/mcp/subscriptions/listen`

## API HTTP

| Método y ruta | Autenticación | Descripción |
| --- | --- | --- |
| `GET /health` | No | Salud básica y tiempo activo. |
| `GET /status` | No | Resumen de estado y configuración del motor. |
| `POST /research` | Sí por TCP | Ejecuta una investigación síncrona. |

Ejemplo:

```bash
curl -X POST http://127.0.0.1:8765/research \
  -H 'Content-Type: application/json' \
  -H 'X-Pharus-Token: <daemon_token>' \
  -d '{"topic":"Computación cuántica","language":"es","profile":"balanced"}'
```

El token está en el campo `daemon_token` de `~/.pharus/config.yaml`. Las
peticiones realizadas mediante el socket Unix omiten la comprobación del token;
las peticiones TCP a `/research` y `/mcp` deben enviar `X-Pharus-Token`. El
cliente también puede enviar `X-Pharus-User-ID`; para conexiones loopback se usa
`user_local_default` si se omite.

## MCP

El demonio registra un manejador Streamable HTTP sin estado en `/mcp` y ofrece:

| Herramienta o recurso | Estado | Descripción |
| --- | --- | --- |
| `deep_research` | Funcional | Investigación síncrona. |
| `start_async_research` | Funcional | Inicia una tarea en memoria y devuelve su URI. |
| `resource://tasks/{id}` | Funcional | Lee el progreso y resultado de una tarea. |
| `resource://tasks/list` | Funcional | Lista las tareas visibles para el usuario. |
| `ask_user_input` | Experimental | Devuelve una estructura propia con estado `input_required`; no pausa ni reanuda automáticamente una llamada. |
| `execute_code_script` | Experimental | Ejecuta Python o Bash con backend nativo o Docker. |
| `vector_search` | Funcional | Consulta evidencia real, aislada por usuario autenticado y opcionalmente por `research_id`. |

Las tareas asíncronas y sus eventos se conservan únicamente en memoria y se
pierden al reiniciar el demonio. El canal SSE
`/mcp/subscriptions/listen` es una extensión HTTP propia, no una suscripción MCP
estándar.

## Configuración

La configuración principal está en `~/.pharus/config.yaml`. Pharus también carga
`.env` desde el directorio actual, junto al ejecutable, en su directorio padre y
desde `~/.pharus/.env`. Las variables del entorno del proceso tienen prioridad
sobre el YAML.

Valores predeterminados relevantes:

| Opción | Valor |
| --- | --- |
| LLM | MiniMax, modelo `MiniMax-M3` |
| Reintentos LLM | 5 intentos, espera inicial de 2 s y máxima de 30 s |
| Embeddings | OMLX en `http://localhost:8000`, modelo `mlx-community/Qwen3-Embedding-0.6B-8bit` |
| SearXNG | `http://localhost:8090` |
| Vectores | `chromem`, colección `pharus_research` |
| Perfil | `balanced` |
| Checkpoints | habilitados, vigencia de 72 horas |

Variables de entorno admitidas:

| Variable | Descripción |
| --- | --- |
| `PHARUS_LANGUAGE` | `auto`, `en` o `es`. |
| `PHARUS_LLM_PROVIDER` | `minimax`, `openai`, `anthropic`, `ollama` o `llamacpp`. |
| `PHARUS_LLM_RETRY_MAX_ATTEMPTS` | Número máximo de intentos. |
| `PHARUS_LLM_RETRY_INITIAL_BACKOFF_SECONDS` | Espera inicial entre reintentos. |
| `PHARUS_LLM_RETRY_MAX_BACKOFF_SECONDS` | Espera máxima entre reintentos. |
| `PHARUS_MINIMAX_API_KEY`, `PHARUS_MINIMAX_GROUP_ID` | Credenciales MiniMax. |
| `PHARUS_MINIMAX_MODEL`, `PHARUS_MINIMAX_BASE_URL`, `PHARUS_MINIMAX_MAX_TOKENS` | Configuración MiniMax. |
| `PHARUS_OPENAI_API_KEY`, `PHARUS_OPENAI_MODEL`, `PHARUS_OPENAI_BASE_URL` | Configuración OpenAI. |
| `PHARUS_ANTHROPIC_API_KEY`, `PHARUS_ANTHROPIC_MODEL`, `PHARUS_ANTHROPIC_BASE_URL` | Configuración Anthropic. |
| `PHARUS_OLLAMA_LLM_URL`, `PHARUS_OLLAMA_LLM_MODEL` | Configuración de chat en Ollama. |
| `PHARUS_EMBEDDING_PROVIDER` | `omlx`, `mlx`, `ollama`, `openai`, `external` o `custom`. |
| `PHARUS_EMBEDDING_URL`, `PHARUS_EMBEDDING_MODEL`, `PHARUS_EMBEDDING_API_KEY` | Configuración de embeddings. |
| `PHARUS_SEARXNG_URL`, `PHARUS_SEARXNG_IMAGE` | Servicio e imagen SearXNG. |
| `PHARUS_VECTOR_PROVIDER`, `PHARUS_VECTOR_COLLECTION` | `chromem` o `lancedb`, y nombre base de colección. |
| `PHARUS_RESEARCH_PROFILE` | `quick`, `balanced` o `deep`. |
| `PHARUS_RESEARCH_RESUME_SESSIONS` | Activa o desactiva checkpoints. |
| `PHARUS_RESEARCH_CHECKPOINT_MAX_AGE_HOURS` | Vigencia de checkpoints. |
| `PHARUS_LLAMACPP_URL`, `PHARUS_LLAMACPP_MODEL`, `PHARUS_LLAMACPP_GRAMMAR_ENABLE` | Proveedor llama.cpp y soporte GBNF. |
| `PHARUS_DATASTORM_SOURCES` | Mapa YAML/JSON de tabla a archivo CSV, por ejemplo `{research_metrics: /datos/metrics.csv}`. |

Los límites HTTP, el backend del sandbox y otras opciones sólo se configuran en
el archivo YAML.

## Almacenamiento vectorial

| Proveedor | Implementación real |
| --- | --- |
| `chromem` | `chromem-go` persistente en `~/.pharus/vectors`. Es el valor predeterminado. |
| `lancedb` | SDK Go upstream `lancedb-go` sobre el formato y motor nativos de LanceDB. Requiere compilar con `-tags lancedb_native` y enlazar los artefactos nativos del SDK. |

El nombre de la colección incorpora el proveedor y modelo de embeddings para
evitar mezclar espacios vectoriales incompatibles. Una compilación sin soporte
nativo rechaza explícitamente `provider: lancedb`; nunca crea un archivo JSON
con extensión engañosa.

DataSTORM se activa desde la CLI y el demonio al configurar una o más fuentes:

```yaml
datastorm:
  sources:
    research_metrics: /ruta/absoluta/metrics.csv
```

La primera fila del CSV define las columnas. Una fuente inválida detiene el
arranque del motor en vez de desactivar DataSTORM silenciosamente.

## Benchmarks

Las suites integradas son pequeñas muestras de humo, no copias oficiales de los
datasets que nombran:

```bash
./bin/pharus benchmark list
./bin/pharus benchmark run --suite synthetic --limit 1
./bin/pharus benchmark run --file dataset.jsonl --output resultados
```

`benchmark run` ejecuta el motor real y usa el LLM configurado como juez RACE,
por lo que consume los servicios y credenciales configurados.

## Desarrollo y pruebas

```bash
go test -short ./...
./dev.sh
```

`dev.sh` instala `air` si no está disponible y ejecuta el demonio con recarga
automática. Sin `-short`, `go test ./...` también puede activar pruebas en vivo
cuando encuentra credenciales. Las pruebas de integración requieren servicios y
credenciales y pueden ejecutarse de forma explícita con:

```bash
./test_llm.sh
```

La carpeta `postman/` contiene una colección y un entorno de ejemplo para la API
local.

## Aislamiento y dependencias opcionales

El sandbox nativo usa `sandbox-exec` en macOS y `bubblewrap` + `prlimit` en
Linux. Aplica política de archivos/red y timeout; Linux también aplica límites
de CPU/memoria. Como macOS no ofrece aquí un límite de memoria por proceso
fiable, el backend nativo rechaza `memory_mb > 0` y exige Docker. Docker añade
filesystem de sólo lectura, límites de procesos, capacidades eliminadas,
memoria, CPU y red. Ambos backends fallan de forma cerrada: Docker nunca ejecuta
el script localmente si el contenedor no puede arrancar.

LanceDB requiere los headers y binarios nativos publicados por
[`lancedb-go`](https://github.com/lancedb/lancedb-go). La compilación estándar
mantiene `chromem` como valor predeterminado y no requiere CGO adicional.

## Licencia

Pharus se distribuye bajo la
[GNU General Public License v3.0](LICENSE).
