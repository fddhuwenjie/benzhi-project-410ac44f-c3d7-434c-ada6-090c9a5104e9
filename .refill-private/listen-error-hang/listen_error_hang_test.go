package listen_error_hang_test

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOccupiedAddressExitsInsteadOfWaitingForSignal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	binary := filepath.Join(t.TempDir(), "relay-server")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/server")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建服务失败: %v: %s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-addr="+listener.Addr().String(), "-data-dir="+t.TempDir())
	err = command.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("监听地址冲突后服务丢弃 ListenAndServe 错误并持续等待信号")
	}
	if err == nil {
		t.Fatal("监听地址冲突后服务以成功状态退出")
	}
}
