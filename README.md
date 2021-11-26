# SIPtoWebRTC

SIP audio to WebBrowser over WebRTC based on Pion and go-sip-ua



## Installation
1.
```bash
$ export GO111MODULE=on
$ go get github.com/liceim/SIPtoWebRTC
```
2.
```bash
$ cd ~/go/src/github.com/liceim/SIPtoWebRTC
```
3.
```bash
$ go run .
```
or
```bash
$ go build .
$ ./SIPtoWebRTC
```
4.
```bash
open web browser http://127.0.0.1:8083 work chrome, safari, firefox
```

## Configuration

### Edit file config.json

format:

```bash
{
  "server": {
    "http_port": ":8083",
    "ice_servers": ["stun:stun.l.google.com:19302"],
    "ice_username": "",
    "ice_credential": ""
  },
  "streams": {
    "sip_phone_400": {
      "on_demand": false,
      "url": "sip:400@127.0.0.1"
    }
  }
}
```

## Limitations

Audio Codecs Supported: opus pcm alaw and pcm mulaw 

