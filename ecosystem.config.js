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
  ],
};
