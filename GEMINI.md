# HydraStream & HydraForge Project Guidelines (GEMINI.md)

Este documento define a arquitetura, design system, catálogo de skills, pipelines de dados/IA, rotinas de testes, validação rigorosa de parâmetros e protocolo de auto-reparo do ecossistema **HydraStream** (Control Plane & Ingestão Zero-Copy) e **HydraForge** (Estúdio de Treinamento YOLO).

---

## 0. Arquitetura Split-Plane & Limites Invioláveis do Ecossistema

O ecossistema Hydra opera em 4 planos com responsabilidades atômicas:

1. **Data Plane / Motor de Mídia (HydraStream :8080 & MediaMTX :8554/:8889):** Exclusivo para descoberta ONVIF WS-Discovery, probing, ingestão RTSP TCP RFC 2326, snapshots e streaming WebRTC WHEP/HLS, além de buffer Zero-Copy `/dev/shm` e CUDA IPC na RTX 5090.
2. **Control Plane (HydraVMS :8083):** Exclusivo para PostgreSQL (câmeras, pastas, usuários, RBAC, layouts, perfis de gravação e workflows). **PROIBIDO** conter processamento de vídeo ou FFmpeg.
3. **Analytics Plane (HydraForge :8081):** Treinamento YOLO, autotuning e compilação TensorRT na RTX 5090.
4. **Curation Plane (HydraVault :8082):** Curadoria e auto-rotulagem de datasets com SAM 2.

---

## 1. Filosofia de Engenharia e Estrutura Arquitetural

O projeto segue estritamente a **Arquitetura Hexagonal (Ports & Adapters)** combinada com **Domain-Driven Design (DDD)**:

```text
hydrastream / hydraforge
├── cmd/                          # Entrypoints principais dos binários
├── internal/
│   ├── domain/                   # Entidades puras e regras de negócio (ZERO imports de infra/HTTP)
│   ├── ports/                    # Interfaces de entrada (Driving) e saída (Driven)
│   ├── application/              # Casos de uso e orquestração de serviços
│   └── adapters/
│       ├── primary/http/         # Controladores REST, gRPC e WebSockets
│       └── secondary/
│           ├── ingest/           # Adaptador RFC 2326 RTSP / TCP RTP
│           ├── gpu/              # Detector de hardware GPU (NVIDIA RTX 5090 / CUDA 13.3)
│           ├── memory/           # Repositório thread-safe em memória
│           ├── worker/           # IPC e controle de processos de treinamento
│           └── shm/              # Gerenciador de memória compartilhada POSIX (/dev/shm)
├── crates/hydra-engine/          # Motor Data Plane em Rust (Lock-free Ring Buffer & Governor)
├── worker_python/                # Worker de treinamento e inferência Ultralytics / PyTorch
├── sdk/python/                   # SDK Python Zero-Copy para analíticos
└── web/                          # Frontend SPA (HTML/CSS/JS ou React)
```

### Regras Inquebráveis de Backend:
1. **Pureza do Domínio:** O pacote `internal/domain/` **NUNCA** deve importar pacotes de infraestrutura como `net/http`, `database/sql` ou bibliotecas externas de rede.
2. **Concorrência Segura:** Toda sessão de stream ou worker é isolada com `context.WithCancel` e sincronizada via ponteiros atômicos (`sync/atomic`) e `sync.RWMutex`.
3. **Hardware Acceleration:** Prioridade máxima para Zero-Copy (`/dev/shm` no CPU e `CUDA IPC` na VRAM da NVIDIA RTX 5090).

---

## 2. Design System & Estilo Visual (Clean Slate Dark / High-Tech HUD)

O frontend adota o design system oficial Clean Slate Dark (espelhado do **HydraVMS**):

### Tokens e Cores Primárias:
- **Fundo Principal & Superfícies:** `#07080c` (Main Background), `#0e1117` (Surface), `#14171d` (Elevated Cards).
- **Cores de Ação e Destaque:**
  - Primário / Ações: `#ff5e3a` (Laranja HUD / Ações Primárias)
  - Bordas Sutis: `rgba(255, 255, 255, 0.06)`
  - Textos Principais: `#ffffff` (Títulos & Dados), `#8b94a0` (Muted / Labels)
- **Status LEDs Universais (Neon Dots sem texto ao lado):**
  - Online / Ingestão Ativa: `#00ff9d` (Verde neon com glow)
  - Fan-Out / Gravação: `#00f0ff` (Azul neon pulsante)
  - Alerta / Buffer Alto: `#fcee0a` (Amarelo cyber neon)
  - Queda / Erro: `#ff003c` (Vermelho magenta neon)
- **Tipografia Modular:** Google Fonts (`Roboto` / `Inter` para interface e `JetBrains Mono` para métricas, IDs, offsets e FPS).

### ⚠️ Regras Estritas de Frontend & UI Standards:
- **Limite Máximo de 100 Linhas por Arquivo:** Nenhum arquivo CSS, JS ou JSX em `web/` pode ultrapassar **100 linhas**. Se crescer, deve ser modularizado.
- **Proibição Absoluta de Emojis em Listas e Dropdowns:** Nunca usar emojis dentro de `<select>`, `<option>`, dropdowns, tabelas ou listas em nenhum projeto.
- **Nomenclatura Concisa & HUD:** Rótulos atômicos (`INGESTAO`, `CONSUMIDORES`, `CODEC`, `RESOLUCAO`, `FPS`, `BITRATE`, `SHM BUFFER`).
- **Botões de Confirmação:** O texto do botão de confirmação e salvamento é estritamente **`SALVAR`**.

---

## 3. Catálogo de Skills & Ecossistema Ultralytics

O projeto possui integração direta com as skills oficiais de IA e UI localizadas em `.agents/skills/`:

| Skill | Finalidade |
| :--- | :--- |
| **`stream-ui`** | Design system Clean Slate Dark, tokens, tipografia (`Roboto`/`JetBrains Mono`), layout e componentes modulares (< 100 linhas). |
| **`validate-project`** | Executa auditoria de conformidade DDD, contagem de linhas web e testes unitários. |
| **`yolo`** | Router principal de comandos e CLI Ultralytics. |
| **`yolo-models`** | Guia de arquiteturas: YOLOv8, YOLO11, YOLO26 (Nano a XLarge) e variantes (`detect`, `segment`, `pose`, `obb`). |
| **`yolo-training`** | Hiperparâmetros, otimizadores (`AdamW`/`SGD`), batch size dinâmico, AMP FP16/BF16 e callbacks. |
| **`yolo-datasets`** | Estruturas de `data.yaml`, classes, splits train/val e anotações. |
| **`yolo-tuning`** | Otimização de hiperparâmetros por algoritmos genéticos e Ray Tune. |
| **`yolo-inference`** | Predição de alta velocidade, tracking e streaming de vídeo. |
| **`yolo-export`** | Compilação e exportação para **TensorRT (`.engine`)**, **ONNX**, FP16 e quantização INT8. |

---

## 4. Pipeline de Ingestão e Treinamento de Modelos

```text
[ Câmera IP / RTSP ]
        │
        ▼ (RFC 2326 TCP Demuxing)
[ HydraStream Ingest Engine ]
        │
        ▼ (Zero-Copy Ring Buffer /dev/shm - 8.46 GB/s)
[ Pipeline Fan-Out & FPS Governor ]
   ├──► [ Consumidor Analítico YOLO (2 FPS) ]
   └──► [ Active Learning / Dataset Collection ]
              │
              ▼ (Treinamento na RTX 5090 via HydraForge)
        [ Ultralytics PyTorch Training Loop ]
              │
              ▼ (Exportação 1-Clique)
        [ TensorRT Engine (.engine) / ONNX ]
              │
              ▼
        [ Inferência Ultrarrápida a 18.500+ FPS ]
```

---

## 5. Validação de Parâmetros e Restrições de Domínio

Antes de qualquer execução ou persistência, os seguintes limites devem ser validados:

1. **Parâmetros de Stream:**
   - `stream_id`: Obrigatório, alfanumérico e underscores (ex: `cam_entrance_01`).
   - `ingest_fps`: Entre `1.0` e `120.0` FPS.
   - `target_fps` por consumidor: Entre `0.1` e `60.0` FPS.
2. **Parâmetros de Treinamento YOLO:**
   - `epochs`: Inteiro positivo entre `1` e `1000`.
   - `imgsz`: Múltiplo de 32 (ex: `640`, `1280`, `1920`).
   - `device`: Identificador CUDA válido (`0`, `cuda:0` ou `cpu`).
   - `batch`: `-1` (Auto-batch) ou potências de 2 (`8`, `16`, `32`, `64`).

---

## 6. Bateria de Testes e Benchmarks Físicos

Todos os comandos de validação são automatizados via `Makefile`:

```bash
# Executa todos os testes unitários e de concorrência (Go & Rust)
make test

# Executa o benchmark de throughput de memória compartilhada POSIX SHM em Rust
make benchmark

# Executa a comparação em hardware real: Tradicional vs HydraStream CPU vs GPU RTX 5090
make benchmark-compare

# Inicia o servidor em modo de desenvolvimento com hot-reload visual
make dev

# Inicia o servidor MediaMTX local embutido
make mediamtx

# Publica fluxo RTSP de teste
make stream-sample
```

---

## 7. Protocolo de Auto-Reparo & Validação Pós-Prompt (Self-Healing Rules)

Ao final de **qualquer modificação de código**, o agente deve obrigatoriamente executar o seguinte checklist de auto-reparo antes de concluir:

```bash
# 1. Auditoria de Limite de Linhas Web (< 100 linhas por arquivo)
wc -l web/css/*.css web/css/*/*.css web/js/*.js web/js/*/*.js web/src/**/*.css web/src/**/*.jsx 2>/dev/null

# 2. Auditoria de Pureza DDD (Nenhum import de net/http no domínio)
grep -rn "net/http" internal/domain/ && echo "VIOLAÇÃO DDD: Remova net/http do domínio!"

# 3. Compilação e Testes Automatizados
make test && make build

# 4. Sincronização da Documentação
# Garantir que README.md (Inglês) e README.pt-BR.md (Português) estejam 100% atualizados.
```

Se qualquer etapa falhar, o código deve ser corrigido imediatamente antes de prosseguir.
