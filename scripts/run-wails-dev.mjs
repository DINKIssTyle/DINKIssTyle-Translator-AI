import { spawn } from "node:child_process";
import { createServer } from "node:net";

const requestedPort = Number.parseInt(process.argv[2] ?? "9245", 10);

if (!Number.isInteger(requestedPort) || requestedPort < 1 || requestedPort > 65535) {
  console.error(`[dev] Invalid Vite port: ${process.argv[2] ?? ""}`);
  process.exit(1);
}

function tryListen(port, host) {
  return new Promise((resolve) => {
    const server = createServer();

    server.unref();
    server.once("error", (error) => {
      server.close();
      resolve(error.code === "EADDRNOTAVAIL" ? null : false);
    });
    server.listen({ port, host, exclusive: true }, () => {
      server.close(() => resolve(true));
    });
  });
}

async function isPortAvailable(port) {
  // Vite resolves localhost to IPv6 on macOS, while other platforms commonly
  // use IPv4. Check both loopback families so Wails and Vite select one port.
  const ipv6 = await tryListen(port, "::1");
  const ipv4 = await tryListen(port, "127.0.0.1");
  return ipv6 !== false && ipv4 !== false;
}

async function findAvailablePort(startPort) {
  for (let port = startPort; port <= Math.min(startPort + 100, 65535); port += 1) {
    if (await isPortAvailable(port)) {
      return port;
    }
  }
  throw new Error(`No available Vite port found between ${startPort} and ${Math.min(startPort + 100, 65535)}`);
}

let port;
try {
  port = await findAvailablePort(requestedPort);
} catch (error) {
  console.error(`[dev] ${error.message}`);
  process.exit(1);
}

if (port !== requestedPort) {
  console.warn(`[dev] Port ${requestedPort} is already in use; using ${port} for Wails and Vite.`);
}

const executable = process.platform === "win32" ? "wails3.exe" : "wails3";
const child = spawn(
  executable,
  ["dev", "-config", "./build/config.yml", "-port", String(port)],
  {
    env: { ...process.env, WAILS_VITE_PORT: String(port) },
    stdio: "inherit",
  },
);

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => {
    if (!child.killed) {
      child.kill(signal);
    }
  });
}

child.once("error", (error) => {
  console.error(`[dev] Unable to start ${executable}: ${error.message}`);
  process.exitCode = 1;
});

child.once("exit", (code, signal) => {
  if (signal) {
    process.exitCode = 130;
    return;
  }
  process.exitCode = code ?? 1;
});
