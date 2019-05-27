package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/fishioon/wxgo/webwx"
	qrcode "github.com/skip2/go-qrcode"
)

func msgHandle(wx *webwx.Wechat, msgs []*webwx.Msg) error {
	for _, msg := range msgs {
		fromUser := wx.GetUser(msg.FromUserName)
		toUser := wx.GetUser(msg.ToUserName)
		if fromUser != nil {
			log.Printf("FromUserName:%s %s", msg.FromUserName, fromUser.NickName)
		}
		if toUser != nil {
			log.Printf("ToUserName:%s %s", msg.ToUserName, toUser.NickName)
		}
		log.Printf("Content:%s", msg.Content)
		log.Printf("MsgType:%d", msg.MsgType)
		if strings.HasPrefix(msg.FromUserName, "@@") {
		} else {
			if msg.FromUserName != msg.ToUserName {
				wx.SendMsg(msg.FromUserName, msg.Content)
			}
		}
	}
	return nil
}

// WebwxRun login webwx and run
func WebwxRun(w http.ResponseWriter, _ *http.Request) {
	png, err := webwxRun()
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("content-type", "image/jpeg")
	w.Write(png)
}

func webwxRun() ([]byte, error) {
	codeURL, err := webwx.NewLoginCodeURL()
	if err != nil {
		return nil, err
	}
	return qrcode.Encode(codeURL, qrcode.Medium, 256)
	// return c.Blob(http.StatusOK, "image/jpeg", png)
}

func handleScript(w http.ResponseWriter, r *http.Request) {
	res, err := runShellCommand(r.URL.Query().Get("script"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(res))
}

func handleLocation(w http.ResponseWriter, r *http.Request) {
	location := r.URL.Query().Get("location")
	if location == "" {
		location = "Automatic"
	}
	res, err := changeNetworkLocation(location)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(res))
}

func runShellCommand(script string) (string, error) {
	cmd := exec.Command("sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
	return string(out), nil
}

func changeNetworkLocation(location string) (string, error) {
	cmd := fmt.Sprintf("networksetup -switchtolocation \"%s\"", location)
	return runShellCommand(cmd)
}
