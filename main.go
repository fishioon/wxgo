package main

import (
	"flag"
	"log"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/fishioon/wxgo/webwx"
)

var (
	Version   string
	BuildTime string
)

type Config struct {
}

func main() {
	listenHost := flag.String("host", "127.0.0.1:9981", "listen host")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\nBuild: %s\n", Version, BuildTime)
		return
	}

	rand.Seed(time.Now().Unix())
	/*
		str := strconv.Itoa(rand.Int())
		deviceID := "e" + str[2:17]
	*/

	// start already login session

	// wait login
	go webwx.Run(msgHandle, "")

	http.HandleFunc("/cmd/shell", handleScript)
	http.HandleFunc("/cmd/network/location", handleLocation)
	http.HandleFunc("/webwx/new", WebwxRun)
	fmt.Printf("login wechat with [http://%s/webwx/new]\n", *listenHost)
	log.Fatal(http.ListenAndServe(*listenHost, nil))
}
