package main

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bendersilver/glog"
	"github.com/go-audio/wav"
	"github.com/go-redis/redis/v8"
	"github.com/imroc/req"
	"github.com/joho/godotenv"
	tb "gopkg.in/tucnak/telebot.v2"
)

const cyclon = "https://cyclon.motorlab.su"

var re = regexp.MustCompile(`CYCLE\d+`)
var ren = regexp.MustCompile(`\d+`)

var bot *tb.Bot
var kdb *redis.Client
var ctx = context.Background()

func main() {

	req.SetTimeout(time.Minute * 10)
	glog.Debug("start kdb:", os.Getenv("KDB_HOST"))
	kdb = redis.NewClient(&redis.Options{
		Addr: os.Getenv("KDB_HOST"),
		DB:   14,
	})
	if kdb.Ping(ctx).Val() != "PONG" {
		glog.Fatal("redis not exists")
	}

	// if !kdb.HExists(ctx, "cyclon.motorlab", "last").Val() {
	kdb.HSet(ctx, "cyclon.motorlab", "last", "CYCLE01").Err()
	// }
	var err error
	bot, err = tb.NewBot(tb.Settings{
		Token:       os.Getenv("BOT_TOKEN"),
		Poller:      &tb.LongPoller{Timeout: 10 * time.Second},
		Synchronous: true,
		ParseMode:   tb.ModeMarkdown,
		Reporter:    func(e error) { glog.Error(e) },
	})
	if err != nil {
		glog.Fatal(err)
	}

	bot.Handle("/start", commandStart)
	loopCyclonMotorlab()
	bot.Start()
}

func commandStart(m *tb.Message) {
	glog.Debug(m.Sender.ID)
	bot.Send(m.Sender, "empty")
}

func download(p string) error {
	rsp, err := req.Get(cyclon + p)
	if err != nil {
		return err
	}
	os.RemoveAll("/tmp/CYCLETMP/")
	os.MkdirAll("/tmp/CYCLETMP/", os.ModePerm)
	fl := "/tmp/CYCLETMP/" + path.Base(p)
	err = rsp.ToFile(fl)
	if err != nil {
		return err
	}
	err = exec.Command("unzip", fl, "-d", "/tmp/CYCLETMP/").Run()
	if err != nil {
		return err
	}

	var alb tb.Album

	err = filepath.Walk("/tmp/CYCLETMP/", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		if path.Ext(info.Name()) == ".wav" {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			s, err := wav.NewDecoder(f).Duration()
			if err != nil {
				return err
			}
			glog.Debug(info.Name(), p, int(s.Seconds()))
			a := &tb.Audio{Title: info.Name(), File: tb.FromDisk(p), Duration: int(s.Seconds())}
			msg, err := bot.Send(&tb.User{ID: 80868958}, a)
			// telegram: Request Entity Too Large (400
			if err != nil {
				return err
			}
			bot.Delete(msg)
			alb = append(alb, a)
		} else if path.Ext(info.Name()) == ".jpg" {
			_, err := bot.Send(&tb.User{ID: 80868958}, &tb.Photo{File: tb.FromDisk(p)})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		glog.Error(err)
		return err
	}
	_, err = bot.SendAlbum(&tb.User{ID: 80868958}, alb)
	return err
}

func loopCyclonMotorlab() {
	var ok bool
	var release, last string
	var releases []string
	for {
		ok = false
		last = kdb.HGet(ctx, "cyclon.motorlab", "last").Val()
		doc, err := goquery.NewDocument(cyclon)
		if err != nil {
			glog.Error(err)
			continue
		}
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			item, ok := s.Attr("href")
			if ok {
				releases = append(releases, item)
			}
		})
		for i := len(releases) - 1; i >= 0; i-- {
			release = re.FindString(path.Base(releases[i]))
			if release != "" {
				if release == last {
					ok = true
				} else if ok {
					glog.Debug(download(releases[i]))
					break
				}
			}

		}
		time.Sleep(time.Minute)
	}
}

func init() {
	err := godotenv.Load()
	if err != nil {
		glog.Fatal(err)
	}
	for _, k := range []string{"BOT_TOKEN", "KDB_HOST"} {
		if _, ok := os.LookupEnv(k); !ok {
			glog.Fatal("set environment variable", k)
		}
	}
}
