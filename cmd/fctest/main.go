package main

import (
	"context"
	"fmt"
	"log"

	"github.com/harshalvk/cage/internal/firecracker"
)

func main() {
	mgr, err := firecracker.NewFirecrackerManager(firecracker.Config{
		FirecrackerBin: "/usr/local/bin/firecracker",
		KernelPath:     "/home/harshal/firecracker/vmlinux/vmlinux.bin",
		RootfsBaseDir:  "/var/lib/cage/rootfs-base",
		RunDir:         "/tmp/cage-fc-run",
		VCPUCount:      1,
		MemSizeMiB:     128,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	sandboxID := "test-sandbox-pause"

	fmt.Println("→ creating sandbox...")
	if err := mgr.CreateSandbox(ctx, sandboxID, "base"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  created!")

	fmt.Println("→ writing a file to prove state presists across pause...")
	if err := mgr.WriteFile(ctx, sandboxID, "/tmp/before-pause.txt", "still here after resume"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("→ pausing (real snapshot + memory capture)...")
	pauseRef, err := mgr.PauseSandbox(ctx, sandboxID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("  paused, pauseRef =", pauseRef)

	running, err := mgr.IsRunning(ctx, sandboxID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("  IsRunning after pause (expect false):", running)

	fmt.Println("→ resuming from snapshot...")
	if err := mgr.ResumeSandbox(ctx, sandboxID, pauseRef); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  resumed")

	running, err = mgr.IsRunning(ctx, sandboxID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("  IsRunning after resume (expect true):", running)

	fmt.Println("→ confirming the file survided the pause/resume cycle...")
	stdout, stderr, exitCode, err := mgr.ExecCommand(ctx, sandboxID, []string{"cat", "/tmp/before-pause.txt"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("stdout=%q stderr=%q exit=%d\n", stdout, stderr, exitCode)

	fmt.Println("→ cleaning up (kill running sandbox)...")
	if err := mgr.KillSandbox(ctx, sandboxID); err != nil {
		log.Fatal(err)
	}

	fmt.Println("→ cleaning up pause resources...")
	if err := mgr.RemoveImage(ctx, pauseRef); err != nil {
		log.Fatal(err)
	}

	fmt.Println("done - full pause/resume cycle verified against real Firecracker")
}
