package operator

import (
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rtctunnel/rtctunnel/channels"
)

func init() {
	channels.RegisterFactory("operator", func(addr string) (channels.Channel, error) {
		return New(strings.Replace(addr, "operator://", "https://", 1)), nil
	})
}

// DefaultClient is the client to use for making http requests
var DefaultClient = &http.Client{
	Timeout: 30 * time.Second,
}

// An operatorChannel signals over a custom http server.
type operatorChannel struct {
	url string
}

// New creates a new operatorChannel.
func New(url string) channels.Channel {
	_, err := DefaultClient.Head(url)
	if err != nil && strings.Contains(err.Error(), "server gave HTTP response") {
		// switch to http
		url = strings.Replace(url, "https://", "http://", 1)
	}
	return &operatorChannel{url: url}
}

// Recv receives a message at the given key.
func (c *operatorChannel) Recv(key string) (data string, err error) {
	log.Debug().Str("url", c.url).Str("key", key).Msg("[operator] receive")

	uv := url.Values{
		"address": {key},
	}
	for {
		resp, err := DefaultClient.Get(c.url + "/sub?" + uv.Encode())
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				log.Warn().Msg("[operator] timed-out, retrying")
				continue
			}
			return "", err
		}
		if resp.StatusCode == http.StatusGatewayTimeout {
			log.Warn().Msg("[operator] timed-out, retrying")
			resp.Body.Close()
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return "", errors.New(resp.Status)
		}

		bs, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		log.Debug().Str("key", key).Str("data", string(bs)).Msg("[operator] received")

		return string(bs), nil
	}
}

// Send sends a message to the given key with the given data.
func (c *operatorChannel) Send(key, data string) error {
	log.Debug().Str("url", c.url).Str("key", key).Str("data", data).Msg("[operator] send")

	uv := url.Values{
		"address": {key},
		"data":    {data},
	}
	for {
		resp, err := DefaultClient.PostForm(c.url+"/pub", uv)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusGatewayTimeout {
			resp.Body.Close()
			continue
		}

		log.Debug().Int("status_code", resp.StatusCode).Str("status", resp.Status).Msg("[operator] sent")

		ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		return nil
	}
}
