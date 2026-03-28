package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/avenstack/pwip/service"
)

var version = "dev"

func main() {
	var (
		configPath  string
		showVersion bool
	)

	flag.StringVar(&configPath, "config", "passwall_config.json", "服务配置文件路径")
	flag.BoolVar(&showVersion, "v", false, "打印版本")
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, "Passwall Preferred IP Service %s\n\n", version)
		fmt.Fprintln(os.Stdout, "用法:")
		fmt.Fprintln(os.Stdout, "  ./app -config passwall_config.json")
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "参数:")
		fmt.Fprintln(os.Stdout, "  -config string")
		fmt.Fprintln(os.Stdout, "      服务配置文件路径 (默认 passwall_config.json)")
		fmt.Fprintln(os.Stdout, "  -v")
		fmt.Fprintln(os.Stdout, "      打印版本")
	}
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		return
	}

	if err := service.Run(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
		os.Exit(1)
	}
}
