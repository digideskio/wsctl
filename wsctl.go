/**
 * WebSocket Command Line Tool
 * (C) Copyright 2015 Daniel-Constantin Mierla (asipto.com)
 * License: GPLv2
 */

package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"golang.org/x/net/websocket"
)

const wsctlVersion = "1.0"

var templateFields = map[string]map[string]interface{}{
	"FIELDS:EMPTY": {},
}

//
// CLIOptions - structure for command line options
type CLIOptions struct {
	wsurl         string
	wsorigin      string
	wsproto       string
	wsinsecure    bool
	wsreceive     bool
	wstemplate    string
	wsfields      string
	wscrlf        bool
	version       bool
	wsauser       string
	wsapasswd     string
	wstimeoutrecv int
	wstimeoutsend int
}

var cliops = CLIOptions{
	wsurl:         "wss://127.0.0.1:8443",
	wsorigin:      "http://127.0.0.1",
	wsproto:       "sip",
	wsinsecure:    true,
	wsreceive:     true,
	wstemplate:    "",
	wsfields:      "",
	wscrlf:        false,
	version:       false,
	wsauser:       "",
	wsapasswd:     "",
	wstimeoutrecv: 20000,
	wstimeoutsend: 10000,
}

//
// initialize application components
func init() {
	// command line arguments
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s (v%s):\n", filepath.Base(os.Args[0]), wsctlVersion)
		fmt.Fprintf(os.Stderr, "    (each option has short and long version)\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	flag.StringVar(&cliops.wsauser, "auser", cliops.wsauser, "username to be used for authentication")
	flag.StringVar(&cliops.wsapasswd, "apasswd", cliops.wsapasswd, "password to be used for authentication")
	flag.BoolVar(&cliops.wscrlf, "crlf", cliops.wscrlf, "replace '\\n' with '\\r\\n' inside the data to be sent (true|false)")
	flag.StringVar(&cliops.wsfields, "fields", cliops.wsfields, "path to the json fields file")
	flag.StringVar(&cliops.wsfields, "f", cliops.wsfields, "path to the json fields file")
	flag.BoolVar(&cliops.wsinsecure, "insecure", cliops.wsinsecure, "skip tls certificate validation for wss (true|false)")
	flag.BoolVar(&cliops.wsinsecure, "i", cliops.wsinsecure, "skip tls certificate validation for wss (true|false)")
	flag.StringVar(&cliops.wsorigin, "origin", cliops.wsorigin, "origin http url")
	flag.StringVar(&cliops.wsorigin, "o", cliops.wsorigin, "origin http url")
	flag.StringVar(&cliops.wsproto, "proto", cliops.wsproto, "websocket sub-protocol")
	flag.StringVar(&cliops.wsproto, "p", cliops.wsproto, "websocket sub-protocol")
	flag.BoolVar(&cliops.wsreceive, "receive", cliops.wsreceive, "wait to receive response from ws server (true|false)")
	flag.BoolVar(&cliops.wsreceive, "r", cliops.wsreceive, "wait to receive response from ws server (true|false)")
	flag.StringVar(&cliops.wstemplate, "template", cliops.wstemplate, "path to template file (mandatory parameter)")
	flag.StringVar(&cliops.wstemplate, "t", cliops.wstemplate, "path to template file (mandatory parameter)")
	flag.StringVar(&cliops.wsurl, "url", cliops.wsurl, "websocket url (ws://... or wss://...)")
	flag.StringVar(&cliops.wsurl, "u", cliops.wsurl, "websocket url (ws://... or wss://...)")
	flag.BoolVar(&cliops.version, "version", cliops.version, "print version")
	flag.IntVar(&cliops.wstimeoutrecv, "timeout-recv", cliops.wstimeoutrecv, "timeout waiting to receive data (milliseconds)")
	flag.IntVar(&cliops.wstimeoutsend, "timeout-send", cliops.wstimeoutsend, "timeout trying to send data (milliseconds)")
}

//
// wsctl application
func main() {

	flag.Parse()

	fmt.Printf("\n")

	if cliops.version {
		fmt.Printf("%s v%s\n", filepath.Base(os.Args[0]), wsctlVersion)
		os.Exit(1)
	}

	// options for ws connections
	urlp, err := url.Parse(cliops.wsurl)
	if err != nil {
		log.Fatal(err)
	}
	orgp, err := url.Parse(cliops.wsorigin)
	if err != nil {
		log.Fatal(err)
	}

	tlc := tls.Config{
		InsecureSkipVerify: false,
	}
	if cliops.wsinsecure {
		tlc.InsecureSkipVerify = true
	}

	// buffer to send over ws connction
	var buf bytes.Buffer
	var tplstr = ""
	if len(cliops.wstemplate) > 0 {
		tpldata, err := ioutil.ReadFile(cliops.wstemplate)
		if err != nil {
			log.Fatal(err)
		}
		tplstr = string(tpldata)
	} else {
		log.Fatal("missing data template file ('-t' or '--template' parameter must be provided)")
	}

	var tplfields interface{}
	if len(cliops.wsfields) > 0 {
		fieldsdata, err := ioutil.ReadFile(cliops.wsfields)
		if err != nil {
			log.Fatal(err)
		}
		err = json.Unmarshal(fieldsdata, &tplfields)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		tplfields = templateFields["FIELDS:EMPTY"]
	}

	var tpl = template.Must(template.New("wsout").Parse(tplstr))
	tpl.Execute(&buf, tplfields)

	var wmsg []byte
	if cliops.wscrlf {
		wmsg = []byte(strings.Replace(buf.String(), "\n", "\r\n", -1))
	} else {
		wmsg = buf.Bytes()
	}

	// open ws connection
	// ws, err := websocket.Dial(wsurl, "", wsorigin)
	ws, err := websocket.DialConfig(&websocket.Config{
		Location:  urlp,
		Origin:    orgp,
		Protocol:  []string{cliops.wsproto},
		Version:   13,
		TlsConfig: &tlc,
		Header:    http.Header{"User-Agent": {"wsctl"}},
	})
	if err != nil {
		log.Fatal(err)
	}

	// send data to ws server
	err = ws.SetWriteDeadline(time.Now().Add(time.Duration(cliops.wstimeoutsend) * time.Millisecond))
	_, err = ws.Write(wmsg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sending (%d bytes):\n[[%s]]\n", len(wmsg), wmsg)

	// receive data from ws server
	if cliops.wsreceive {
		var rmsg = make([]byte, 8192)
		err = ws.SetReadDeadline(time.Now().Add(time.Duration(cliops.wstimeoutrecv) * time.Millisecond))
		n, err := ws.Read(rmsg)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Receiving (%d bytes):\n[[%s]]\n", n, rmsg)
		if n > 24 && cliops.wsproto == "sip" {
			ManageSIPResponse(ws, wmsg, rmsg)
		}
	}
}

//
// ParseAuthHeader - parse www/proxy-authenticate header body.
// Return a map of parameters or nil if the header is not Digest auth header.
func ParseAuthHeader(hbody []byte) map[string]string {
	s := strings.SplitN(strings.Trim(string(hbody), " "), " ", 2)
	if len(s) != 2 || s[0] != "Digest" {
		return nil
	}

	params := map[string]string{}
	for _, kv := range strings.Split(s[1], ",") {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		params[strings.Trim(parts[0], "\" ")] = strings.Trim(parts[1], "\" ")
	}
	return params
}

//
// BuildAuthResponseHeader - return the body for auth header in response
func BuildAuthResponseHeader(username string, password string, hparams map[string]string) string {
	// https://en.wikipedia.org/wiki/Digest_access_authentication
	// HA1
	h := md5.New()
	A1 := fmt.Sprintf("%s:%s:%s", username, hparams["realm"], password)
	io.WriteString(h, A1)
	HA1 := fmt.Sprintf("%x", h.Sum(nil))

	// HA2
	h = md5.New()
	A2 := fmt.Sprintf("%s:%s", hparams["method"], hparams["uri"])
	io.WriteString(h, A2)
	HA2 := fmt.Sprintf("%x", h.Sum(nil))

	AuthHeader := ""
	if _, ok := hparams["qop"]; !ok {
		// build digest response
		response := HMD5(strings.Join([]string{HA1, hparams["nonce"], HA2}, ":"))
		// build header body
		AuthHeader = fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=MD5, response="%s"`,
			username, hparams["realm"], hparams["nonce"], hparams["uri"], response)
	} else {
		// build digest response
		cnonce := RandomKey()
		response := HMD5(strings.Join([]string{HA1, hparams["nonce"], "00000001", cnonce, hparams["qop"], HA2}, ":"))
		// build header body
		AuthHeader = fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", cnonce="%s", nc=00000001, qop=%s, opaque="%s", algorithm=MD5, response="%s"`,
			username, hparams["realm"], hparams["nonce"], hparams["uri"], cnonce, hparams["qop"], hparams["opaque"], response)
	}
	return AuthHeader
}

//
// RandomKey - return random key (used for cnonce)
func RandomKey() string {
	key := make([]byte, 12)
	for b := 0; b < len(key); {
		n, err := rand.Read(key[b:])
		if err != nil {
			panic("failed to get random bytes")
		}
		b += n
	}
	return base64.StdEncoding.EncodeToString(key)
}

//
// HMD5 - return a lower-case hex MD5 digest of the parameter
func HMD5(data string) string {
	md5d := md5.New()
	md5d.Write([]byte(data))
	return fmt.Sprintf("%x", md5d.Sum(nil))
}

//
// ManageSIPResponse - process a SIP response
// - if was a 401/407, follow up with authentication request
func ManageSIPResponse(ws *websocket.Conn, wmsg []byte, rmsg []byte) bool {
	if cliops.wsapasswd == "" {
		return false
	}
	// www or proxy authentication
	hname := ""
	if bytes.HasPrefix(rmsg, []byte("SIP/2.0 401 ")) {
		hname = "WWW-Authenticate:"
	} else if bytes.HasPrefix(rmsg, []byte("SIP/2.0 407 ")) {
		hname = "Proxy-Authenticate:"
	}
	n := bytes.Index(rmsg, []byte(hname))
	if n < 0 {
		return false
	}
	hbody := bytes.Trim(rmsg[n:n+bytes.Index(rmsg[n:], []byte("\n"))], " \t\r")
	hparams := ParseAuthHeader(hbody[len(hname):])
	if hparams == nil {
		return false
	}
	auser := "test"
	if cliops.wsauser != "" {
		auser = cliops.wsauser
	}

	s := strings.SplitN(string(wmsg), " ", 3)
	if len(s) != 3 {
		return false
	}

	hparams["method"] = s[0]
	hparams["uri"] = s[1]
	fmt.Printf("\nAuth params map:\n    %+v\n\n", hparams)
	authResponse := BuildAuthResponseHeader(auser, cliops.wsapasswd, hparams)

	// build new request - increase CSeq and insert auth header
	n = bytes.Index(wmsg, []byte("CSeq:"))
	if n < 0 {
		n = bytes.Index(wmsg, []byte("s:"))
		if n < 0 {
			return false
		}
	}
	hbody = bytes.Trim(wmsg[n:n+bytes.Index(wmsg[n:], []byte("\n"))], " \t\r")
	var obuf bytes.Buffer
	obuf.Write(wmsg[:n])
	s = strings.SplitN(string(hbody), " ", 3)
	if len(s) != 3 {
		return false
	}
	csn, _ := strconv.Atoi(s[1])
	cs := strconv.Itoa(1 + csn)

	obuf.WriteString("CSeq: " + cs + " " + s[2] + "\r\n")
	if hname[0] == 'W' {
		obuf.WriteString("Authorization: ")
	} else {
		obuf.WriteString("Proxy-Authorization: ")
	}
	obuf.WriteString(authResponse)
	obuf.WriteString("\r\n")
	obuf.Write(wmsg[1+n+bytes.Index(wmsg[n:], []byte("\n")):])

	// sending data to ws server
	_, err := ws.Write(obuf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Resending (%d bytes):\n[[%s]]\n", obuf.Len(), obuf.Bytes())

	// receive data from ws server
	if cliops.wsreceive {
		var imsg = make([]byte, 8192)
		n, err := ws.Read(imsg)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Receiving: (%d bytes)\n[[%s]]\n", n, imsg)
	}

	return true
}
