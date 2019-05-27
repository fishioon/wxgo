package webwx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/golang/glog"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

const (
	httpUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/47.0.2526.111 Safari/537.36"
	httpReferer   = "https://wx.qq.com/"
	cgipath       = "https://wx.qq.com/cgi-bin/mmwebwx-bin"
	syncHost      = "webpush.wx.qq.com"
)

const (
	cmdNewLogin = "uuid"
	cmdLoginOK  = "loginok"
	cmdRunning  = "runingok"
)

var (
	cmdChan = make(chan Cmd)
	wechats = make(map[string]*Wechat)
)

type Cmd struct {
	command string
	content string
}

// User user info
type User struct {
	Uin         int    `json:"Uin"`
	UserName    string `json:"UserName"`
	NickName    string `json:"NickName"`
	HeadImgURL  string `json:"HeadImgUrl"`
	ContactFlag int    `json:"ContactFlag"`
	RemarkName  string `json:"RemarkName"`
	Sex         int    `json:"Sex"`
	Status      int    `json:"Status"`
	AttrStatus  int    `json:"AttrStatus"`
	Province    string `json:"Province"`
	City        string `json:"City"`
}

// Wechat wechat context
type Wechat struct {
	Skey        string `json:"skey"`
	Uin         string `json:"uid"`
	Sid         string `json:"sid"`
	DeviceID    string `json:"device_id"`
	PassTicket  string `json:"pass_ticket"`
	UUID        string `json:"uuid"`
	Name        string
	baseRequest map[string]interface{}
	contacts    map[string]User
	client      *http.Client
}

// Session wechat login user session

// Msg wechat msg struct
type Msg struct {
	MsgID        string `json:"MsgId"`
	FromUserName string `json:"FromUserName"`
	ToUserName   string `json:"ToUserName"`
	MsgType      int    `json:"MsgType"`
	Content      string `json:"Content"`
	Status       string `json:"Status"`
}

type kv struct {
	Key int `json:"Key"`
	Val int `json:"Val"`
}

type syncKey struct {
	Count int  `json:"Count"`
	List  []kv `json:"List"`
}

type getContactRes struct {
	MemberCount int    `json:"MemberCount"`
	MemberList  []User `json:"MemberList"`
}

func packSyncKey(syncKeys []kv) string {
	var keys []string
	for _, e := range syncKeys {
		keys = append(keys, fmt.Sprintf("%d_%d", e.Key, e.Val))
	}
	return strings.Join(keys, "|")
}

// MsgHandleFunc webwx recv msg will call
type MsgHandleFunc func(wx *Wechat, msgs []*Msg) error

// SendMsg send msg
func (w *Wechat) SendMsg(toUserName, content string) (err error) {
	return w.sendmsg(toUserName, content)
}

func (w *Wechat) GetUser(username string) *User {
	u, ok := w.contacts[username]
	if !ok {
		return nil
	}
	return &u
}

func (w *Wechat) msgRecv(sk *syncKey, handle MsgHandleFunc, argv interface{}) (err error) {
	var (
		retcode, selector string
		tryTimes          int
		msgs              []*Msg
		res               []byte
	)
	for {
		if retcode, selector, err = w.synccheck(sk); err != nil || retcode != "0" {
			if tryTimes > 3 {
				break
			}
			time.Sleep(time.Second)
			tryTimes++
			continue
		}
		tryTimes = 0
		if res, err = w.sync(sk); err != nil {
			break
		}

		json.Get(res, "SyncKey").ToVal(sk)
		json.Get(res, "AddMsgList").ToVal(&msgs)

		if selector == "2" && err == nil {
			if err = handle(w, msgs); err != nil {
				break
			}
		} else if selector == "0" {
			time.Sleep(time.Millisecond * 1000)
		} else {
			err = fmt.Errorf("unknow selector: %s", selector)
			break
		}
	}
	glog.Errorf("stop recv msg, err:%s", err.Error())
	return
}

func (w *Wechat) getcontact() (err error) {
	r := time.Now().UnixNano() / int64(time.Millisecond)
	url := fmt.Sprintf("%s/webwxgetcontact?r=%d&seq=0&skey=%s", cgipath, r, w.Skey)
	params := map[string]interface{}{
		"BaseRequest": w.baseRequest,
	}
	body, err := w.wxpost(url, params)
	if err != nil {
		return
	}
	res := new(getContactRes)
	if err = json.Unmarshal(body, res); err == nil {
		for _, user := range res.MemberList {
			w.contacts[user.UserName] = user
		}
	}
	return
}

func (w *Wechat) synccheck(sk *syncKey) (string, string, error) {
	skstr := packSyncKey(sk.List)
	r := time.Now().UnixNano() / int64(time.Millisecond)
	url := fmt.Sprintf("https://%s/cgi-bin/mmwebwx-bin/synccheck?r=%d&skey=%s&sid=%s&uin=%s&deviceid=%s&synckey=%s&_=%d",
		syncHost, r, w.Skey, url.QueryEscape(w.Sid), w.Uin, w.DeviceID, skstr, r)
	res, err := w.wxget(url)
	if err != nil {
		return "", "", err
	}
	// resp: window.synccheck={retcode:"0",selector:"0"}
	re := regexp.MustCompile(`window.synccheck={retcode:"(\d+)",selector:"(\d+)"}`)
	find := re.FindStringSubmatch(res)
	return find[1], find[2], err
}

func (w *Wechat) sync(sk *syncKey) ([]byte, error) {
	url := fmt.Sprintf("%s/webwxsync?sid=%s&skey=%s&pass_ticket=%s", cgipath, w.Sid, w.Skey, w.PassTicket)
	params := map[string]interface{}{
		"BaseRequest": w.baseRequest,
		"SyncKey":     sk,
		"rr":          ^int(time.Now().Unix()),
	}
	return w.wxpost(url, params)
}

func (w *Wechat) sendmsg(toUserName, content string) (err error) {
	r := time.Now().UnixNano() / int64(time.Millisecond)
	msgid := fmt.Sprintf("%d%04d", r, rand.Intn(10000))
	params := map[string]interface{}{
		"BaseRequest": w.baseRequest,
		"Msg": map[string]interface{}{
			"Type":         1,
			"Content":      content,
			"FromUserName": w.Name,
			"ToUserName":   toUserName,
			"LocalID":      msgid,
			"ClientMsgID":  msgid,
		},
	}
	urlstr := fmt.Sprintf("%s/webwxsendmsg?pass_ticket=%s", cgipath, w.PassTicket)
	_, err = w.wxpost(urlstr, params)
	return
}

func (w *Wechat) wxget(urlstr string) (result string, err error) {
	return httpClientGet(w.client, urlstr)
}

func (w *Wechat) wxpost(urlstr string, params map[string]interface{}) (result []byte, err error) {
	glog.V(2).Infof("url:%s", urlstr)
	var (
		jsonb   []byte
		request *http.Request
	)
	if jsonb, err = json.Marshal(params); err != nil {
		return
	}
	requestBody := bytes.NewBuffer([]byte(jsonb))
	if request, err = http.NewRequest("POST", urlstr, requestBody); err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json;charset=utf-8")
	request.Header.Add("Referer", httpReferer)
	request.Header.Add("User-agent", httpUserAgent)
	resp, err := w.client.Do(request)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if result, err = ioutil.ReadAll(resp.Body); err != nil {
		return
	}
	glog.V(2).Infof("result:%s", string(result))
	return
}

func waitLogin(uuid string) error {
	var (
		res, url, code string
		err            error
		r              int64
		tip            = -1
	)
	for {
		tip++
		r = time.Now().UnixNano() / int64(time.Millisecond)
		url = fmt.Sprintf("%s/login?loginicon=true&tip=%d&uuid=%s&_=%d", cgipath, tip, uuid, r)
		res, err = httpGet(url)
		if err == nil {
			code = res[12:15]
		}
		if code == "200" {
			idx := strings.Index(res, "window.redirect_uri=")
			if idx == -1 {
				err = errors.New("wait confirm login invalid response")
				return err
			}
			idx += len("window.redirect_uri=") + 1
			loginCodeURL := res[idx : len(res)-2]

			cookieJar, _ := cookiejar.New(nil)
			client := &http.Client{
				Jar: cookieJar,
			}

			if res, err = httpClientGet(client, loginCodeURL+"&fun=new&version=v2"); err != nil {
				return err
			}

			session := struct {
				Skey       string `xml:"skey"`
				Sid        string `xml:"wxsid"`
				Uin        string `xml:"wxuin"`
				PassTicket string `xml:"pass_ticket"`
			}{}

			if err = xml.Unmarshal([]byte(res), &session); err != nil {
				glog.Errorf("invalid session data: %s err: %s", res, err.Error())
				return err
			}

			w := &Wechat{
				Skey:       session.Skey,
				Sid:        session.Sid,
				Uin:        session.Uin,
				PassTicket: session.PassTicket,
				UUID:       uuid,
			}
			w.Init(client)

			return nil
		} else if code == "408" {
			return errors.New("login timeout")
		} else if code == "201" {
			time.Sleep(time.Second * 1)
		} else {
			time.Sleep(time.Second * 1)
		}
	}
}

/*
func NewWechatFromSessionData(data string) (*Wechat, error) {
	w := new(Wechat)
	var err error
	if err = json.Unmarshal([]byte(data), w); err != nil {
		return nil, err
	}
	w.Init()
	return w, nil
}
*/

func (w *Wechat) SaveSession() string {
	return ""
}

func (w *Wechat) Init(client *http.Client) {
	if w.DeviceID == "" {
		w.DeviceID = "e786342593651293"
	}
	w.baseRequest = map[string]interface{}{
		"Uin":      w.Uin,
		"Sid":      w.Sid,
		"Skey":     w.Skey,
		"DeviceID": w.DeviceID,
	}
	w.contacts = make(map[string]User)
	w.client = client

	wechats[w.Uin] = w
	sendCmd(cmdLoginOK, w.Uin)
}

func (w *Wechat) start(handle MsgHandleFunc, argv string) error {
	var (
		res []byte
		err error
	)

	// 1. webwxinit
	params := map[string]interface{}{
		"BaseRequest": w.baseRequest,
	}
	r := time.Now().UnixNano() / int64(time.Millisecond)
	url := fmt.Sprintf("%s/webwxinit?pass_ticket=%s&r=%d", cgipath, w.PassTicket, r)
	if res, err = w.wxpost(url, params); err != nil {
		return err
	}
	w.Name = json.Get(res, "User", "UserName").ToString()
	sk := new(syncKey)
	json.Get(res, "SyncKey").ToVal(sk)

	// 2. webwxstatusnotify
	params = map[string]interface{}{
		"BaseRequest":  w.baseRequest,
		"Code":         3,
		"FromUserName": w.Name,
		"ToUserName":   w.Name,
		"ClientMsgId":  int(time.Now().Unix()),
	}
	urlstr := fmt.Sprintf("%s/webwxstatusnotify?lang=zh_CN&pass_ticket=%s", cgipath, w.PassTicket)
	if res, err = w.wxpost(urlstr, params); err != nil {
		return err
	}
	retcode := json.Get(res, "BaseResponse", "Ret").ToInt()
	if retcode != 0 {
		err = fmt.Errorf("statusnotify retcode:%d", retcode)
	}

	// 3. getcontact
	r = time.Now().UnixNano() / int64(time.Millisecond)
	url = fmt.Sprintf("%s/webwxgetcontact?r=%d&seq=0&skey=%s", cgipath, r, w.Skey)
	params = map[string]interface{}{
		"BaseRequest": w.baseRequest,
	}
	if res, err = w.wxpost(url, params); err != nil {
		return err
	}
	contacts := new(getContactRes)
	if err = json.Unmarshal(res, contacts); err == nil {
		for _, user := range contacts.MemberList {
			w.contacts[user.UserName] = user
		}
	}

	// 4. handleMessage
	return w.msgRecv(sk, handle, argv)
}

func sendCmd(cmd, content string) {
	cmdChan <- Cmd{cmd, content}
}

func Run(handle MsgHandleFunc, argv string) error {
	for {
		select {
		case cmd := <-cmdChan:
			switch cmd.command {
			case cmdNewLogin: // uuid
				go waitLogin(cmd.content)
			case cmdLoginOK: // wechat uin
				w, _ := wechats[cmd.content]
				go w.start(handle, argv)
			case cmdRunning:
			}
		}
	}
	return nil
}

func NewLoginCodeURL() (string, error) {
	r := time.Now().UnixNano() / int64(time.Millisecond)
	url := fmt.Sprintf("https://login.weixin.qq.com/jslogin?appid=wx782c26e4c19acffb&redirect_uri=%s&fun=new&lang=zh_CN&_=%d", "", r)
	res, err := httpGet(url)
	if err != nil {
		return "", err
	}
	begin := strings.Index(res, "\"")
	end := strings.Index(res[begin+1:], "\"")
	uuid := res[begin+1 : begin+end+1]

	// add uuid to wait login queue
	sendCmd(cmdNewLogin, uuid)

	codeURL := "https://login.weixin.qq.com/l/" + uuid
	return codeURL, nil
}

func httpClientGet(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		glog.Errorf("http get fail err=%s", err.Error())
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	glog.V(2).Infof("url: %s\n res: %s", url, string(body))
	return string(body), nil
}

func httpGet(url string) (string, error) {
	return httpClientGet(http.DefaultClient, url)
}
