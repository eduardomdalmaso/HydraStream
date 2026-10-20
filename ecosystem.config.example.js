const path = require("path");

/**
 * HydraStream PM2 Ecosystem Template (Exemplo Multiplataforma)
 * Copie este arquivo para `ecosystem.config.js` para customizações locais.
 * Suporta Windows e Linux automaticamente.
 */

module.exports = {
  apps: [
    {
      name: "hydra-stream",
      cwd: __dirname,
      script: process.platform === "win32" ? "./hydrastream.exe" : "./bin/hydrastream",
      interpreter: "none",
      autorestart: true,
      max_restarts: 10,
      restart_delay: 2000,
      env: {
        PORT: "8080",
        JWT_SECRET: "hydravms-enterprise-secure-jwt-signing-key-32b-secret",
      },
    },
  ],
};
