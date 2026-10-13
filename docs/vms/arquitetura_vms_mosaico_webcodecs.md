# Arquitetura de Alta Performance para Mosaicos de Vídeo em VMS Modernos
## Da Ingestão C++/Rust ao WebCodecs + WebGL no Navegador

**Data:** 13/10/2026  
**Módulo:** VMS / Streaming / Mosaico  
**Referência:** Análise de engenharia reversa e arquitetura do Vezha VMS (IncoreSoft) e padrões industriais (Milestone, Genetec, Nx Witness).

---

## 1. O Problema das Abordagens Tradicionais de Mosaico Web

Em plataformas de videomonitoramento (CFTV/VMS), um dos maiores desafios é exibir grades de **16, 24, 32 ou 64 câmeras simultâneas** em um navegador sem congelar a interface do operador ou sobrecarregar o servidor.

### Por que a abordagem convencional falha?
A maioria dos projetos web inicia usando tags HTML padrão:
```html
<video src="..." autoplay playsinline></video>
```
1. **Esgotamento de Sessões de Hardware do SO:**
   - Cada tag `<video>` cria um pipeline multimídia completo no sistema operacional (DirectShow/Media Foundation no Windows, AVFoundation no Mac).
   - As GPUs dedicadas e integradas possuem um limite rígido de instâncias de decodificação simultâneas. Ao abrir mais de 8 a 12 vídeos, os novos canais começam a apresentar tela preta, engasgos e erros de decodificação.
2. **Dessincronização com Inteligência Artificial (Bounding Boxes):**
   - Quando o servidor envia caixas delimitadoras de IA (rostos, placas, detecção de pessoas) via WebSocket e o vídeo via WebRTC/HLS convencional, a latência do player de vídeo e do WebSocket varia.
   - O resultado é o efeito "fantasma": o retângulo verde da IA anda na frente ou atrás da pessoa.
3. **Overhead de Negociação WebRTC (SDP/ICE/DTLS):**
   - Criar 32 conexões `RTCPeerConnection` completas consome dezenas de portas UDP, trocas de candidatos ICE e sobrecarrega o roteamento de rede do cliente.

---

## 2. A Arquitetura Industrial Moderna (Vezha / VMS de Alta Escala)

Os sistemas de missão crítica modernos abandonaram as tags `<video>` para mosaicos e adotaram uma arquitetura de **Pipeline Desacoplado (Decodificação e Renderização em Hardware via W3C WebCodecs)**:

```
+-----------------------------------------------------------------------------------+
|                                SERVIDOR DE VÍDEO (NVR)                            |
|                                                                                   |
|  [ Câmera IP ]                                                                    |
|       |                                                                           |
|    (RTSP)                                                                         |
|       v                                                                           |
|  [ Ingestão C++/Rust ] ---> [ Gravação Disco: Blocos 1GB .dat + Índice LSM-Tree ] |
|       |                     (Mainstream 4K/1080p - Alta Resolução)                |
|       |                                                                           |
|       +-------------------> [ Empacotador WebSocket / WSP ]                       |
|                             (Substream 640x360 - Baixa Resolução / Leve)          |
+---------------------------------------+-------------------------------------------+
                                        |
                               (WebSocket WSS / WSP)
                                        |
+---------------------------------------v-------------------------------------------+
|                                NAVEGADOR DO CLIENTE                               |
|                                                                                   |
|  [ WebSocket Receiver (ArrayBuffer) ]                                             |
|       |                                                                           |
|       v                                                                           |
|  [ WebCodecs API: new VideoDecoder() ] <--- Usa GPU Local (Aceleração Hardware)   |
|       |                                                                           |
|       v (VideoFrame)                                                              |
|  [ HTML5 <canvas> + WebGL ] <-------------- Renderiza Vídeo a 60 FPS              |
|       ^                                                                           |
|       |                                                                           |
|  [ Metadados de IA (Bounding Boxes) ] <---- Desenha Molduras no mesmo ciclo       |
+-----------------------------------------------------------------------------------+
```

---

## 3. Os Pilares da Arquitetura

### Pilar A: Dual-Streaming Dinâmico (Mainstream vs. Substream)
- **Mainstream (Fluxo Principal):** 1080p ou 4K a 30 FPS (4 Mbps a 8 Mbps).
  - É direcionado **estritamente para a gravação contínua em disco** (`/data/storages`).
- **Substream (Fluxo Secundário):** 640x360 ou 720x480 a 15 FPS (300 Kbps a 500 Kbps).
  - É o fluxo entregue ao mosaico.
  - **Matemática de Rede:** 16 câmeras em Substream consomem juntas menos de **8 Mbps** de banda, contra mais de **80 Mbps** se usassem o fluxo principal.
- **Chaveamento Dinâmico:** Se o operador der duplo clique em um quadrante para maximizar em tela cheia, o frontend solicita a troca transparente daquele canal para o Mainstream.

---

### Pilar B: Transporte via WSP (WebSocket Stream Protocol)
Em vez de sobrecarregar o cliente com negociações WebRTC complexas para cada miniatura da tela:
- O backend abre uma conexão **WebSocket segura (WSS)**.
- O servidor de gravação repassa diretamente os pacotes **H.264 NALUs** (Network Abstraction Layer Units) envelopados com um cabeçalho binário ultra-enxuto:
  - Timestamp (4 ou 8 bytes).
  - Tipo de Frame (`0x01` para Keyframe / I-Frame, `0x02` para P-Frame / Delta).
  - Payload H.264 puro.

---

### Pilar C: Decodificação em Hardware via WebCodecs (`VideoDecoder`)
O motor JavaScript do frontend não utiliza bibliotecas de decodificação de software em WebAssembly (como JSMpeg/libvpx), que sobrecarregam a CPU. Ele delega a decodificação diretamente para o chip gráfico da máquina do operador:

```javascript
// Inicialização do Decodificador de Vídeo em Hardware
const decoder = new VideoDecoder({
    output: (videoFrame) => {
        // Recebe o frame decodificado da GPU
        renderToCanvas(videoFrame);
        videoFrame.close(); // Liberação imediata da memória de vídeo
    },
    error: (err) => console.error("Erro no VideoDecoder:", err)
});

// Configuração do Codec H.264 Baseline/Main (obtido do cabeçalho SPS/PPS)
decoder.configure({
    codec: "avc1.42001f", // H.264 Baseline Profile nível 3.1
    optimizeForLatency: true
});
```

---

### Pilar D: Renderização em `<canvas>` com WebGL
Cada quadrante do mosaico é uma tag `<canvas>` HTML5:
1. O `VideoFrame` gerado pelo `VideoDecoder` é transferido como textura para o **WebGL** ou desenhado via `CanvasRenderingContext2D`.
2. Como o canvas é um contexto gráfico programável, as **Bounding Boxes de IA**, nomes de pessoas reconhecidas, placas de veículos e alertas de cerca virtual são desenhados diretamente na mesma camada, garantindo **sincronização exata frame-a-quadro**.

---

## 4. Implementação de Referência (Código Minimalista Completo)

Abaixo está o esqueleto funcional de um player WebCodecs de alta performance:

```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <title>Mosaico WebCodecs Player</title>
    <style>
        .grid-mosaico {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 4px;
            background: #111;
            padding: 4px;
        }
        canvas {
            width: 100%;
            height: auto;
            background: #000;
            border-radius: 4px;
        }
    </style>
</head>
<body>

<div class="grid-mosaico">
    <canvas id="camera-1"></canvas>
    <!-- Adicionar múltiplos canvases para o mosaico -->
</div>

<script>
class CanvasStreamPlayer {
    constructor(canvasId, wsUrl) {
        this.canvas = document.getElementById(canvasId);
        this.ctx = this.canvas.getContext('2d');
        this.wsUrl = wsUrl;
        this.initDecoder();
        this.connect();
    }

    initDecoder() {
        this.decoder = new VideoDecoder({
            output: (frame) => {
                // Ajusta dimensões do canvas conforme resolução real da câmera
                if (this.canvas.width !== frame.displayWidth) {
                    this.canvas.width = frame.displayWidth;
                    this.canvas.height = frame.displayHeight;
                }

                // Desenha o frame decodificado
                this.ctx.drawImage(frame, 0, 0);

                // Desenho de Bounding Box de Exemplo (IA Sincronizada)
                this.drawAIOverlay();

                // Libera o frame para não vazar memória de vídeo
                frame.close();
            },
            error: (e) => console.error("Decoder error:", e)
        });

        // Configuração do codec H.264
        this.decoder.configure({
            codec: "avc1.640028", // H.264 High Profile
            optimizeForLatency: true
        });
    }

    drawAIOverlay() {
        this.ctx.strokeStyle = "#00FF00";
        this.ctx.lineWidth = 2;
        this.ctx.strokeRect(50, 50, 120, 160); // Exemplo de rosto/placa
    }

    connect() {
        this.ws = new WebSocket(this.wsUrl);
        this.ws.binaryType = "arraybuffer";

        this.ws.onmessage = (event) => {
            const buffer = new Uint8Array(event.data);
            
            // O primeiro byte define se é Keyframe (0x01) ou Delta (0x02)
            const isKeyframe = buffer[0] === 0x01;
            const chunkData = buffer.slice(1);

            const chunk = new EncodedVideoChunk({
                type: isKeyframe ? "key" : "delta",
                timestamp: performance.now() * 1000,
                data: chunkData
            });

            this.decoder.decode(chunk);
        };
    }
}

// Inicializa a câmera 1
const player1 = new CanvasStreamPlayer('camera-1', 'wss://servidor:2443/ws/live/camera1');
</script>
</body>
</html>
```

---

## 5. Tabela Comparativa: Abordagens de Mosaico Web

| Critério | Tags `<video>` Tradicionais | WebRTC Direto (32 Peers) | WebCodecs + WebSocket (Vezha) |
| :--- | :--- | :--- | :--- |
| **Limite de Câmeras na Tela** | 8 a 12 (trava o navegador) | 16 a 20 (alto uso de CPU/portas) | **36 a 64+ câmeras fluidas** |
| **Consumo de CPU no Cliente** | Alto | Médio/Alto | **Mínimo (GPU Nativa)** |
| **Overhead de Rede** | Alto | Médio | **Mínimo (Substream puro)** |
| **Complexidade de Portas/NAT** | N/A | Complexo (ICE/STUN/TURN) | **Simples (Porta 443 HTTPS/WSS)** |
| **Sincronismo de IA (Bounding Box)** | Dessincronizado | Parcialmente alinhado | **100% Sincronizado no mesmo Frame** |
| **Suporte a Playback Reverso** | Quase impossível | Complexo | **Nativo (servidor envia frames)** |

---

## 6. Conclusão e Recomendações para a Stack K-Monitor / Própria

1. **Para Live Streaming Individual (1 Câmera em Alta Qualidade):**
   - **WebRTC / WHEP** com o **MediaMTX** continua sendo uma solução excepcional e de baixíssima latência (< 200ms).
2. **Para Grades de Mosaico (16+ Câmeras Simultâneas):**
   - Implementar o padrão **Substream + WebSocket Binário + WebCodecs API (`VideoDecoder`)** renderizado em `<canvas>`.
3. **Para Gravação e Perícia Forense no Backend:**
   - Ingestão em **C++** ou **Rust** gravando blocos sequenciais `.dat` de tamanho fixo com índices **LSM-Tree** (RocksDB/WAL), permitindo scrubbing instantâneo e playback reverso sem dependência de containers MP4 comuns.
