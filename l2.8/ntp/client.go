package ntp

import (
	"fmt"
	"time"
	"os"
	"github.com/beevik/ntp"
)

func GetTime(server string) (time.Time, error) {
	if server == "" {
		server = "pool.ntp.org"
	}
	return ntp.Time(server)
}

func Run() {
	t, err := GetTime("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения времени: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(t)
}