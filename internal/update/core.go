package update

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/91xusir/octopus/internal/conf"
	"github.com/91xusir/octopus/internal/utils/shutdown"
	"github.com/charmbracelet/log"
)

func UpdateCore() error {
	log.Infof("start update core")

	filename, err := getDownloadFilename()
	if err != nil {
		log.Warnf("update core failed: %v", err)
		return err
	}

	downloadUrl := updateUrl + "/" + filename
	log.Infof("download url: %s", downloadUrl)
	data, err := doRequestWithFallback(downloadUrl)
	if err != nil {
		log.Warnf("download failed: %v", err)
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		log.Warnf("get executable path failed: %v", err)
		return err
	}
	execName := filepath.Base(execPath)

	tmpDir, err := os.MkdirTemp("", execName+"-update-*")
	if err != nil {
		log.Warnf("create temp dir failed: %v", err)
		return err
	}
	defer os.RemoveAll(tmpDir)
	log.Infof("using temp dir: %s", tmpDir)

	if err := unzip(data, tmpDir); err != nil {
		log.Warnf("unzip failed: %v", err)
		return err
	}

	// 归档内的可执行文件名按 APP_NAME 取, 与运行中的文件名无关: 改名前的旧安装可能仍叫 octopus.exe。
	archiveExec := conf.APP_NAME
	if runtime.GOOS == "windows" {
		archiveExec += ".exe"
	}
	newExec := filepath.Join(tmpDir, archiveExec)
	if info, err := os.Stat(newExec); err != nil || info.IsDir() {
		log.Warnf("new executable not found at %s: %v", newExec, err)
		return fmt.Errorf("new executable not found in archive root: %w", err)
	}
	log.Infof("new executable: %s", newExec)

	oldPath := execPath + ".old"
	if err := os.Rename(execPath, oldPath); err != nil {
		log.Warnf("rename old executable failed: %v", err)
		return err
	}

	if err := copyFile(newExec, execPath); err != nil {
		log.Errorf("replace executable failed, try to restore: %v", err)
		_ = os.Rename(oldPath, execPath)
		return err
	}

	if info, statErr := os.Stat(oldPath); statErr == nil {
		_ = os.Chmod(execPath, info.Mode().Perm())
	}
	_ = os.RemoveAll(oldPath)

	log.Infof("update core success")
	go restartExecutable(execPath)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// getDownloadFilename 返回当前平台对应的发布归档名称, 矩阵与 scripts/build.sh 的发布目标一致。
func getDownloadFilename() (string, error) {
	arch := runtime.GOARCH
	goos := runtime.GOOS

	switch goos {
	case "windows":
		if arch == "amd64" {
			return conf.APP_NAME + "-windows-amd64.zip", nil
		}
	case "darwin":
		switch arch {
		case "amd64":
			return conf.APP_NAME + "-darwin-amd64.zip", nil
		case "arm64":
			return conf.APP_NAME + "-darwin-arm64.zip", nil
		}
	case "linux":
		switch arch {
		case "amd64":
			return conf.APP_NAME + "-linux-amd64.zip", nil
		case "arm64":
			return conf.APP_NAME + "-linux-arm64.zip", nil
		}
	}
	return "", fmt.Errorf("unsupported platform: %s/%s", goos, arch)
}

func restartExecutable(execPath string) {
	shutdown.Shutdown()

	log.Infof("restarting: %q %q", execPath, os.Args[1:])

	if runtime.GOOS == "windows" {
		cmd := exec.Command(execPath, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Errorf("restarting failed: %v", err)
		}
		os.Exit(0)
	}

	if err := syscall.Exec(execPath, os.Args, os.Environ()); err != nil {
		log.Errorf("restarting failed: %v", err)
	}
}
