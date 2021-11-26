package main

import (
	"SIPtoWebRTC/mock"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwebrtc/go-sip-ua/pkg/account"
	"github.com/cloudwebrtc/go-sip-ua/pkg/media/rtp"
	"github.com/cloudwebrtc/go-sip-ua/pkg/session"
	"github.com/cloudwebrtc/go-sip-ua/pkg/stack"
	"github.com/cloudwebrtc/go-sip-ua/pkg/ua"
	"github.com/cloudwebrtc/go-sip-ua/pkg/utils"
	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/sip/parser"
	"github.com/liceim/vdk/av"
	"github.com/liceim/vdk/codec"
	"github.com/pixelbender/go-sdp/sdp"
)

const (
	SignalStreamRTPStop = iota
	SignalCodecUpdate
)

const (
	VIDEO = "video"
	AUDIO = "audio"
)

const (
	H264 = "H264"
	H265 = "H265"
	PCMU = "PCMU"
	PCMA = "PCMA"
	OPUS = "OPUS"
)

const (
	RTPHeaderSize = 12
)

var (
	logger              log.Logger
	udp                 *rtp.RtpUDPStream
	OutgoingPacketQueue chan *av.Packet
	UAagent             *ua.UserAgent
	Profile             *account.Profile
	CallSession         *session.Session
	Callid              string
)

func init() {
	logger = utils.NewLogrusLogger(log.InfoLevel, "Client", nil)
	//logger = utils.NewLogrusLogger(log.DebugLevel, "Client", nil)
}

func createUdp() *rtp.RtpUDPStream {

	udp = rtp.NewRtpUDPStream("127.0.0.1", rtp.DefaultPortMin, rtp.DefaultPortMax, func(data []byte, raddr net.Addr) {
		//logger.Infof("Rtp recevied: %v, laddr %s : raddr %s", len(data), udp.LocalAddr().String(), raddr)
		var retmap []*av.Packet
		var duration time.Duration
		//av.OPUS:
		duration = time.Duration(20) * time.Millisecond
		retmap = append(retmap, &av.Packet{
			Data:            data,
			CompositionTime: time.Duration(1) * time.Millisecond,
			Duration:        duration,
			IsKeyFrame:      false,
		})
		for _, i2 := range retmap {
			OutgoingPacketQueue <- i2
		}
		//logger.Infof("Echo rtp to %v", raddr)
		//udp.Send(data, dest)
	})

	go udp.Read()

	return udp
}

func serveSIP() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	stack := stack.NewSipStack(&stack.SipStackConfig{
		UserAgent:  "Go Sip Client/example-client",
		Extensions: []string{"replaces", "outbound"},
		Dns:        "8.8.8.8"})

	listen := "0.0.0.0:5080"
	logger.Infof("Listen => %s", listen)

	if err := stack.Listen("udp", listen); err != nil {
		logger.Panic(err)
	}

	if err := stack.Listen("tcp", listen); err != nil {
		logger.Panic(err)
	}

	if err := stack.ListenTLS("wss", "0.0.0.0:5091", nil); err != nil {
		logger.Panic(err)
	}

	UAagent = ua.NewUserAgent(&ua.UserAgentConfig{
		SipStack: stack,
	})

	// Todo:
	OutgoingPacketQueue = make(chan *av.Packet)
	done := false

	UAagent.InviteStateHandler = func(sess *session.Session, req *sip.Request, resp *sip.Response, state session.Status) {
		logger.Infof("InviteStateHandler: state => %v, type => %s", state, sess.Direction())

		switch state {
		case session.InviteSent:
			fallthrough
		case session.Answered:
		case session.Confirmed:
			if done {
				break
			}
			logger.Infof(sess.RemoteSdp())
			sesdp, err := sdp.ParseString(sess.RemoteSdp())
			if err != nil {
				logger.Error(err)
				break
			}
			ParseCodec(Callid, sesdp)
			ch := Config.sstGe(Callid)
			ch <- string(session.Confirmed)
			done = true
		case session.InviteReceived:
			udp = createUdp()
			udpLaddr := udp.LocalAddr()
			sdp := mock.BuildLocalSdp(udpLaddr.IP.String(), udpLaddr.Port)
			sess.ProvideAnswer(sdp)
			sess.Accept(200)
		case session.Canceled:
			fallthrough
		case session.Failure:
			fallthrough
		case session.Terminated:
			Config.coDe(Callid)
			udp.Close()
			done = false
		}
	}

	UAagent.RegisterStateHandler = func(state account.RegisterState) {
		logger.Infof("RegisterStateHandler: user => %s, state => %v, expires => %v", state.Account.AuthInfo.AuthUser, state.StatusCode, state.Expiration)
	}

	uri, err := parser.ParseUri("sip:100@127.0.0.1")
	if err != nil {
		logger.Error(err)
	}

	Profile = account.NewProfile(uri.Clone(), "goSIP/example-client",
		&account.AuthInfo{
			AuthUser: "100",
			Password: "100",
			Realm:    "127.0.0.1",
		},
		1800,
		stack,
	)

	recipient, err := parser.ParseSipUri("sip:100@127.0.0.1;transport=udp")
	if err != nil {
		logger.Error(err)
	}

	register, _ := UAagent.SendRegister(Profile, recipient, Profile.Expires, nil)
	time.Sleep(time.Second * 3)

	<-stop

	register.SendRegister(0)

	UAagent.Shutdown()
}

func ParseCodec(name string, mediaSDP *sdp.Session) {

	var ch int
	var CodecData []av.CodecData

	for _, media := range mediaSDP.Media {
		if media.Type != VIDEO && media.Type != AUDIO {
			continue
		}

		if media.Type == VIDEO {
			for _, fm := range media.Format {
				if fm.Name == H264 {
					continue
				} else if fm.Name == H265 {
					continue
				} else {
					logger.Infof("SDP Video Codec Type Not Supported", fm.Name)
				}
			}
		}
		if media.Type == AUDIO {
			for _, fm := range media.Format {
				var ac av.AudioCodecData
				switch strings.ToUpper(fm.Name) {
				case OPUS:
					var cl av.ChannelLayout
					switch fm.Channels {
					case 1:
						cl = av.CH_MONO
					case 2:
						cl = av.CH_STEREO
					default:
						cl = av.CH_MONO
					}
					ac = codec.NewOpusCodecData(fm.ClockRate, cl)
				case PCMU:
					ac = codec.NewPCMMulawCodecData()
				case PCMA:
					ac = codec.NewPCMAlawCodecData()
				case av.PCM.String():
					ac = codec.NewPCMCodecData()
				default:
					logger.Infof("Audio Codec", fm.Name, "not supported")
				}
				if ac != nil {
					CodecData = append(CodecData, ac)
				}
			}
		}
		ch += 2
	}

	Config.coAd(name, CodecData)
}

/*
func SIPWorkerLoop(name, uri string) {
	defer Config.RunUnlock(name)
	for {
		logger.Infof("Stream Try Call peer SIP", name)
		err := SIPWorker(name, uri)
		if err != nil {
			logger.Error(err)
			Config.LastError = err
		}
		if !Config.HasViewer(name) {
			logger.Error(ErrorStreamExitNoViewer)
			return
		}
		time.Sleep(1 * time.Second)
	}
}
*/
func SIPWorker(name, uri string) error {
	logger.Infof("Stream Try Call peer SIP", name)
	udp = createUdp()
	Callid = name
	called, err := parser.ParseUri(uri)
	if err != nil {
		logger.Error(err)
	}

	recipient, err := parser.ParseSipUri(uri + ";" + "transport=udp")
	if err != nil {
		logger.Error(err)
	}

	udpLaddr := udp.LocalAddr()
	//Sip gateway sdp(mock)
	s := mock.BuildLocalSdp(udpLaddr.IP.String(), udpLaddr.Port)

	se := make(chan *session.Session) // Declare a unbuffered channe
	go UAagent.Invite(Profile, called, recipient, &s)

	for {
		select {
		case CallSession = <-se:
			if CallSession == nil {
				break
			}
			logger.Infof(CallSession.RemoteSdp())
		case packetAV := <-OutgoingPacketQueue:
			Config.cast(Callid, *packetAV)
		}
	}
}
