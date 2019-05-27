package main

import (
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/fishioon/wxgo/webwx"
	"github.com/labstack/echo"
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
func WebwxRun(c echo.Context) (err error) {
	codeURL, err := webwx.NewLoginCodeURL()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	png, err := qrcode.Encode(codeURL, qrcode.Medium, 256)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "qrcode encode failed")
	}
	return c.Blob(http.StatusOK, "image/jpeg", png)
}

func handleScript(c echo.Context) (err error) {
	var res string
	if res, err = runShellCommand(c.QueryParam("script")); err != nil {
		return
	}
	return c.JSON(http.StatusOK, res)
}

func runShellCommand(script string) (string, error) {
	cmd := exec.Command("sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
	return string(out), nil
}
