interface Sandbox {
  id: string;
  status: "running" | "paused" | "stopped";
  created_at: string;
  expires_at: string;
  template_slug: string;
}

interface ExecResult {
  stdout: string;
  stderr: string;
  exit_code: number;
}

const server = process.env.CAGE_SERVER ?? "http://localhost:8080";
const apiKey = process.env.CAGE_API_KEY;

if (!apiKey) {
  console.error(
    "Error: set CAGE_API_KEY (generate one with 'make genkey' on the server)",
  );
  process.exit(1);
}

async function cageFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${server}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });

  if (!res.ok) {
    const body = await res.text();
    throw new Error(`Cage API error (${res.status}): ${body}`);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

async function main() {
  console.log("→ Creating a Node sandbox...");
  const sandbox = await cageFetch<Sandbox>("/sandboxes", {
    method: "POST",
    body: JSON.stringify({ template: "node-20" }),
  });
  console.log(`  created: ${sandbox.id}`);

  console.log("→ Running a command inside it...");
  const result = await cageFetch<ExecResult>(`/sandboxes/${sandbox.id}/exec`, {
    method: "POST",
    body: JSON.stringify({ cmd: ["node", "-e", "console.log(2 + 2)"] }),
  });
  process.stdout.write(result.stdout);

  console.log("→ Cleaning up...");
  await cageFetch<void>(`/sandboxes/${sandbox.id}`, { method: "DELETE" });

  console.log("Done.");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
