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
	sandboxID := "test-sandbox-1"

	fmt.Println("creating sandbox...")
	if err := mgr.CreateSandbox(ctx, sandboxID, "base"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("created!")

	stdout, stderr, exitCode, err := mgr.ExecCommand(ctx, sandboxID, []string{"echo", "hello from go wrapper"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("stdout=%q stderr=%q exit=%d\n", stdout, stderr, exitCode)

	fmt.Println("killing sandbox...")
	if err := mgr.KillSandbox(ctx, sandboxID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("done")
}
