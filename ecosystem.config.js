module.exports = {
  apps: [
    // 1. HydraStream - Control Plane, Ingestão Zero-Copy (/dev/shm) & MediaMTX Embutido (porta 8080 & RTSP 8554)
    {
      name: "hydra-stream",
      cwd: "/home/hades/Documents/HydraStream",
      script: "./bin/hydrastream",
      interpreter: "none",
      autorestart: true,
      max_restarts: 10,
      restart_delay: 2000,
      env: {
        PORT: "8080",
      },
    },

    // 2. HydraForge - Estúdio de Treinamento YOLO & RTX 5090 (porta 8081)
    {
      name: "hydra-forge",
      cwd: "/home/hades/Documents/HydraForge",
      script: "./bin/hydraforge",
      interpreter: "none",
      autorestart: true,
      max_restarts: 10,
      restart_delay: 2000,
      env: {
        PORT: "8081",
        CUDA_VISIBLE_DEVICES: "0",
      },
    },

    // 3. HydraVault - Curadoria, Auto-Rotulagem & Edge Cases (porta 8082)
    {
      name: "hydra-vault",
      cwd: "/home/hades/Documents/HydraVault",
      script: "./bin/hydravault",
      interpreter: "none",
      autorestart: true,
      max_restarts: 10,
      restart_delay: 2000,
      env: {
        PORT: "8082",
      },
    },

    // 4. HydraVMS Backend - Control Plane REST API & WebSockets (porta 8083)
    {
      name: "hydra-vms-api",
      cwd: "/home/hades/Documents/HydraVMS",
      script: "./bin/hydravms",
      interpreter: "none",
      autorestart: true,
      max_restarts: 10,
      restart_delay: 2000,
      env: {
        PORT: "8083",
        DATABASE_URL: "postgres://postgres:postgres@127.0.0.1:5432/hydravms?sslmode=disable",
        NATS_URL: "nats://127.0.0.1:4222",
        MINIO_ENDPOINT: "127.0.0.1:9000",
      },
    },

    // 5. HydraVMS Frontend - Painel de Controle e Orquestrador Web (porta 5173)
    {
      name: "hydra-vms",
      cwd: "/home/hades/Documents/HydraVMS/web",
      script: "npm",
      args: "run dev",
      autorestart: true,
      max_restarts: 10,
      restart_delay: 2000,
      env: {
        PORT: "5173",
      },
    },
  ],
};
