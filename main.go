package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"golang.org/x/net/html/charset"
	"io"
	"net/url"
	"os"
)

type Getter interface {
	Get(url string) (string, error)
}

type Processor interface {
	Process(source string) (string, error)
}

type FilterConfig struct {
	Start string `json:"content-start"`
	End   string `json:"content-end"`
}

type FormatConfig struct {
	DeletePatterns []string `json:"deletes"`
}

type SourceConfig struct {
	Url         string `json:"url"`
	NextPattern string `json:"next"`
	AutoFetch   bool   `json:"auto-fetch"`
}

type Config struct {
	Source   SourceConfig `json:"source"`
	Filter   FilterConfig `json:"filter"`
	Formater FormatConfig `json:"formater"`
}

type getter struct {
	Retry int
}

func (g *getter) Get(url string) (string, error) {
	var err error
	var resp *http.Response
	var content []byte
	client := http.DefaultClient
	for cnt := 0; cnt < g.Retry; cnt++ {
		fmt.Println(url)
		resp, err = client.Get(url)
		if nil != err {
			continue
		}
		if resp.StatusCode != 200 {
			err = errors.New(fmt.Sprintf("failed to access the url: %s with code: %d\n", url, resp.StatusCode))
			continue
		}
		break
	}

	if nil != err {
		return "", err
	}

	defer resp.Body.Close()

	var reader io.Reader
	reader, err = charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if nil != err {
		return "", err
	}
	content, err = ioutil.ReadAll(reader)
	return string(content), err
}

type formatter struct {
}

func (f *formatter) Process(source string) (string, error) {
	buff := bytes.NewBufferString("")
	escape := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '<' {
			escape = true
		}
		if !escape {
			buff.WriteByte(c)
		}
		if c == '>' {
			escape = false
		}
	}
	return buff.String(), nil
}

type filter struct {
	ContentStart string
	ContentEnd   string
}

func (f *filter) Process(source string) (string, error) {
	posStart := strings.Index(source, f.ContentStart)
	if posStart < 0 {
		return "", fmt.Errorf("failed to locate content-start: %s", f.ContentStart)
	}
	posStart = posStart + len(f.ContentStart)
	posEnd := strings.Index(source[posStart:], f.ContentEnd)
	if posEnd < 0 {
		return "", fmt.Errorf("failed to locate content-end: %s", f.ContentEnd)
	}

	return source[posStart : posStart+posEnd], nil
}

type urlParser struct {
	BaseUrl    string
	UrlPattern string
}

func (u *urlParser) Process(source string) (string, error) {
	urlPattern := strings.Split(u.UrlPattern, "|")
	if len(urlPattern) != 2 {
		return "", errors.New("URL pattern splits by '|'")
	}
	posStart := strings.Index(source, urlPattern[0])
	if posStart < 0 {
		return "", fmt.Errorf("Failed to match the start of url pattern: %s", urlPattern[0])
	}
	posStart = posStart + len(urlPattern[0])
	posEnd := strings.Index(source[posStart:], urlPattern[1])
	if posEnd < 0 {
		return "", fmt.Errorf("Failed to match the end of url pattern: %s", urlPattern[1])
	}
	rawUrl := source[posStart : posStart+posEnd]
	urlStr := ""
	if strings.HasPrefix(rawUrl, "http") {
		urlStr = rawUrl
	} else if strings.HasPrefix(rawUrl, "/") {
		url, err := url.Parse(u.BaseUrl)
		if nil != err {
			return "", err
		}
		urlStr = url.Hostname() + rawUrl
	} else {
		if len(u.BaseUrl) == 0 || strings.Index(u.BaseUrl, "/") < 0 {
			return "", fmt.Errorf("Can not acquire base url from: %s", u.BaseUrl)
		}

		urlStr = u.BaseUrl[:strings.LastIndex(u.BaseUrl, "/")] + "/" + rawUrl
	}
	return urlStr, nil
}

func main() {
	//unmashal config from stdin
	reader := bufio.NewReader(os.Stdin)
	configBytes, err := ioutil.ReadAll(reader)
	if nil != err {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	config := Config{}
	err = json.Unmarshal(configBytes, &config)
	if nil != err {
		fmt.Println(err.Error())
		os.Exit(2)
	}

	url := config.Source.Url

	for {
		// get the resource
		getter := getter{Retry: 3}
		var content string
		content, err = getter.Get(url)
		if nil != err {
			fmt.Println(err.Error())
			os.Exit(3)
		}

		// next url parse
		urlParser := urlParser{BaseUrl: config.Source.Url, UrlPattern: config.Source.NextPattern}
		url, err = urlParser.Process(content)
		if nil != err {
			fmt.Println("error: ", err.Error())
			os.Exit(4)
		}

		filter := filter{ContentStart: config.Filter.Start, ContentEnd: config.Filter.End}

		data, filterErr := filter.Process(content)
		if nil != filterErr {
			fmt.Println("error: ", filterErr.Error())
			os.Exit(5)
		}

		formatter := formatter{}
		data, filterErr = formatter.Process(data)
		fmt.Println(data)
		if "" == url || !config.Source.AutoFetch {
			break
		}
	}
}

func tryRead(url string, retry int) (string, error) {
	return "", nil
}

func processText(text string) (string, error) {

	text, err := filterText(text)
	return convertText(text), err

}

func nextLink(text string) (string, error) {
	next, err := content(text, `<a id="pt_next" href="`, `">下一章</a>`)
	if nil != err {
		return "", err
	} else {
		return next, nil
	}
}

func nextLink2(pattern string) (string, error) {
	return "", nil
}

func convertText(text string) string {
	tmp := strings.Replace(text, "<br />", "\n", -1)
	return strings.Replace(tmp, "&nbsp;", " ", -1)
}

func filterText(text string) (string, error) {

	var b strings.Builder
	b.WriteString("###########")
	if c, e := content(text, `<title>`, `</title>`); nil != e {
		return "", e
	} else {
		b.WriteString(c)
	}
	b.WriteString("###########\n\n")

	if c, e := content(text, `<div id="nr1">`, `<p class="chapter-page-info">`); nil != e {
		return "", e
	} else {
		b.WriteString(c)
	}

	b.WriteString("\n\n")
	return b.String(), nil
}

func content(text string, start string, end string) (string, error) {
	posStart := strings.Index(text, start)
	if posStart < 0 {
		return "", errors.New(fmt.Sprintf("failed to find the start tag: %s, from: %s", start, text))
	}
	posEnd := strings.Index(text, end)
	if posEnd < 0 {
		return "", errors.New(fmt.Sprintf("failed to find the end tag: %s, from: %s", end, text))
	}
	posStart += len(start)
	return text[posStart:posEnd], nil
}
