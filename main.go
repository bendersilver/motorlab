package main

import (
	"archive/zip"
	"context"
	"os"
	"path"
	"regexp"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bendersilver/glog"
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

	download("/tmp/CYCLE42_Signalbuilder_-_CYCLE42.zip")
	glog.Fatal()
	req.SetTimeout(time.Minute * 10)
	glog.Debug("start kdb:", os.Getenv("KDB_HOST"))
	kdb = redis.NewClient(&redis.Options{
		Addr: os.Getenv("KDB_HOST"),
		DB:   14,
	})
	if !kdb.HExists(ctx, "cyclon.motorlab", "last").Val() {
		kdb.HSet(ctx, "cyclon.motorlab", "last", "CYCLE41").Err()
	}
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
	bot.Start()
}

func commandStart(m *tb.Message) {
	glog.Debug(m.Sender.ID)
	bot.Send(m.Sender, "empty")
}

func download(p string) error {
	// rsp, err := req.Get(cyclon + p)
	// if err != nil {
	// 	return err
	// }
	fl := "/tmp/" + path.Base(p)
	// err := rsp.ToFile(fl)
	// if err != nil {
	// 	return err
	// }
	archive, err := zip.OpenReader(fl)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, f := range archive.File {
		glog.Debug(f.FileInfo().IsDir())
	}
	return nil
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
