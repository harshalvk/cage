import os
import sys
import requests

server = os.environ.get("CAGE_SERVER", "http://localhost:8080")
api_key = os.environ.get("CAGE_API_KEY")

if not api_key:
    print("Error: set CAGE_API_KEY (generate one with 'make genkey' on the server)", file=sys.stderr)
    sys.exit(1)

headers = {"Authorization": f"Bearer {api_key}"}

print("→ Creating a Python sandbox...")
resp = requests.post(f"{server}/sandboxes", json={"template": "python-3.12"}, headers=headers)
resp.raise_for_status()
sandbox = resp.json()
sandbox_id = sandbox["id"]
print(f"  created: {sandbox_id}")

print("→ Running a command inside it...")
resp = requests.post(
    f"{server}/sandboxes/{sandbox_id}/exec",
    json={"cmd": ["python3", "-c", "print(2 + 2)"]},
    headers=headers,
)
resp.raise_for_status()
result = resp.json()
print(result["stdout"], end="")

print("→ Cleaning up...")
resp = requests.delete(f"{server}/sandboxes/{sandbox_id}", headers=headers)
resp.raise_for_status()
print("Done.")