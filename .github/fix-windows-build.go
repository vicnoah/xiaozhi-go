//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// 这个程序提供一个简单的方式来构建Windows版本
// 它会尝试各种构建方式，并提供详细错误信息
func main() {
	if runtime.GOOS != "windows" {
		fmt.Println("这个脚本只能在Windows环境下运行")
		os.Exit(1)
	}

	// 获取命令行参数
	args := os.Args
	outName := "xiaozhi-client-windows.exe"
	version := "dev"

	if len(args) > 1 {
		outName = args[1]
	}
	if len(args) > 2 {
		version = args[2]
	}

	// 获取当前目录和MSYS2目录
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("获取当前目录失败: %v\n", err)
		os.Exit(1)
	}

	msys2Path := "C:\\msys64"
	mingwPath := filepath.Join(msys2Path, "mingw64")

	// 检查MSYS2安装
	if _, err := os.Stat(mingwPath); os.IsNotExist(err) {
		fmt.Printf("找不到MSYS2/MinGW目录: %s\n", mingwPath)
		fmt.Println("请确保已安装MSYS2并安装了必要的包")
		os.Exit(1)
	}

	// 检查必要的库文件
	checkLibraries(mingwPath)

	// 设置环境变量
	os.Setenv("CGO_ENABLED", "1")
	os.Setenv("CGO_CFLAGS", fmt.Sprintf("-I%s\\include -I%s\\include\\opus", mingwPath, mingwPath))
	os.Setenv("CGO_LDFLAGS", fmt.Sprintf("-L%s\\lib -lopus -lportaudio -lwinmm", mingwPath))

	// 添加MSYS2/MinGW bin到PATH
	path := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s\\bin;%s", mingwPath, path))

	// 输出环境信息
	fmt.Println("======= 环境信息 =======")
	fmt.Printf("当前目录: %s\n", wd)
	fmt.Printf("MinGW路径: %s\n", mingwPath)
	fmt.Printf("CGO_CFLAGS: %s\n", os.Getenv("CGO_CFLAGS"))
	fmt.Printf("CGO_LDFLAGS: %s\n", os.Getenv("CGO_LDFLAGS"))

	// 尝试不同的构建方法
	buildSuccess := false

	// 方法1: 标准构建
	fmt.Println("\n======= 尝试标准构建 =======")
	if runBuild(outName, version, nil) {
		buildSuccess = true
		fmt.Println("标准构建成功!")
	} else {
		fmt.Println("标准构建失败，尝试其他方法...")
	}

	// 方法2: nogui构建
	if !buildSuccess {
		fmt.Println("\n======= 尝试nogui构建 =======")
		if runBuild(outName, version, []string{"-tags=nogui"}) {
			buildSuccess = true
			fmt.Println("nogui构建成功!")
		} else {
			fmt.Println("nogui构建失败，尝试其他方法...")
		}
	}

	// 方法3: static构建
	if !buildSuccess {
		fmt.Println("\n======= 尝试static构建 =======")
		if runBuild(outName, version, []string{"-tags=static"}) {
			buildSuccess = true
			fmt.Println("static构建成功!")
		} else {
			fmt.Println("static构建失败，尝试其他方法...")
		}
	}

	// 方法4: 超详细构建
	if !buildSuccess {
		fmt.Println("\n======= 尝试超详细构建 =======")
		if runBuild(outName, version, []string{"-x", "-v", "-a"}) {
			buildSuccess = true
			fmt.Println("超详细构建成功!")
		} else {
			fmt.Println("超详细构建失败...")
		}
	}

	if buildSuccess {
		fmt.Printf("\n构建成功! 输出文件: %s\n", outName)
		os.Exit(0)
	} else {
		fmt.Println("\n所有构建方法都失败了.")
		fmt.Println("请确保已安装所有依赖，并检查上面的错误信息。")
		os.Exit(1)
	}
}

// 检查必要的库文件
func checkLibraries(mingwPath string) {
	fmt.Println("======= 检查库文件 =======")

	// 检查opus库
	libOpusPath := filepath.Join(mingwPath, "lib", "libopus.a")
	if _, err := os.Stat(libOpusPath); os.IsNotExist(err) {
		fmt.Printf("警告: 找不到opus库: %s\n", libOpusPath)
		// 寻找其他可能的文件名
		searchLibrary(mingwPath, "lib", "*opus*.a")
	} else {
		fmt.Printf("opus库已找到: %s\n", libOpusPath)
	}

	// 检查portaudio库
	libPortaudioPath := filepath.Join(mingwPath, "lib", "libportaudio.a")
	if _, err := os.Stat(libPortaudioPath); os.IsNotExist(err) {
		fmt.Printf("警告: 找不到portaudio库: %s\n", libPortaudioPath)
		// 寻找其他可能的文件名
		searchLibrary(mingwPath, "lib", "*portaudio*.a")
	} else {
		fmt.Printf("portaudio库已找到: %s\n", libPortaudioPath)
	}

	// 检查opus头文件
	opusHeaderPath := filepath.Join(mingwPath, "include", "opus", "opus.h")
	if _, err := os.Stat(opusHeaderPath); os.IsNotExist(err) {
		fmt.Printf("警告: 找不到opus头文件: %s\n", opusHeaderPath)
		// 寻找其他可能的位置
		searchLibrary(mingwPath, "include", "opus.h")
	} else {
		fmt.Printf("opus头文件已找到: %s\n", opusHeaderPath)
	}

	// 检查portaudio头文件
	portaudioHeaderPath := filepath.Join(mingwPath, "include", "portaudio.h")
	if _, err := os.Stat(portaudioHeaderPath); os.IsNotExist(err) {
		fmt.Printf("警告: 找不到portaudio头文件: %s\n", portaudioHeaderPath)
		// 寻找其他可能的位置
		searchLibrary(mingwPath, "include", "portaudio.h")
	} else {
		fmt.Printf("portaudio头文件已找到: %s\n", portaudioHeaderPath)
	}
}

// 搜索库文件或头文件
func searchLibrary(basePath, subdir, pattern string) {
	dir := filepath.Join(basePath, subdir)
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		fmt.Printf("搜索 %s 时出错: %v\n", pattern, err)
		return
	}

	if len(matches) == 0 {
		fmt.Printf("在 %s 中找不到匹配 %s 的文件\n", dir, pattern)
	} else {
		fmt.Printf("找到可能的匹配项:\n")
		for _, match := range matches {
			fmt.Printf("  - %s\n", match)
		}
	}
}

// 运行go build命令
func runBuild(outName, version string, extraArgs []string) bool {
	fmt.Printf("构建输出文件: %s 版本: %s\n", outName, version)

	args := []string{"build", "-o", outName, "-ldflags", fmt.Sprintf("-X main.Version=%s", version)}
	if extraArgs != nil {
		args = append(args, extraArgs...)
	}
	args = append(args, "./cmd/client")

	fmt.Printf("执行: go %s\n", strings.Join(args, " "))

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("构建失败: %v\n", err)
		return false
	}

	return true
}
