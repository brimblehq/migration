package ui

import "fmt"

func PrintBanner(onlyBanner ...bool) {
	banner := `
    ██████╗ ██████╗ ██╗███╗   ███╗██████╗ ██╗     ███████╗
    ██╔══██╗██╔══██╗██║████╗ ████║██╔══██╗██║     ██╔════╝
    ██████╔╝██████╔╝██║██╔████╔██║██████╔╝██║     █████╗  
    ██╔══██╗██╔══██╗██║██║╚██╔╝██║██╔══██╗██║     ██╔══╝  
    ██████╔╝██║  ██║██║██║ ╚═╝ ██║██████╔╝███████╗███████╗
    ╚═════╝ ╚═╝  ╚═╝╚═╝╚═╝     ╚═╝╚═════╝ ╚══════╝╚══════╝
    `
	onlyBannerValue := false
	if len(onlyBanner) > 0 {
		onlyBannerValue = onlyBanner[0]
	}

	if !onlyBannerValue {
		usage := `
        Usage:
            brimble --license-key=<key> [--config=<path>]
        
        Options:
            --license-key   Your Brimble license key (required)
            --config        Path to configuration file (default: ./config.json)
        
        Examples:
            brimble --license-key=XXXX-XXXX-XXXX-XXXX
            brimble --license-key=XXXX-XXXX-XXXX-XXXX --config=./my-config.json
        
        For support: hello@brimble.app
        Documentation: https://docs.brimble.app
        `
		fmt.Printf("\033[1;36m%s\033[0m\n%s", banner, usage)
		return
	}

	fmt.Printf("\033[1;36m%s\033[0m\n", banner)
}
