package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const MAX_CROUTINE = 5
const Neteasymp3url = "http://music.163.com/api/song/enhance/download/url?br=320000&id="

type TDownload struct {
	filename string
	songname string
	songlink string
}

func worker(jobs <-chan TDownload, rets chan<- string) {
	for job := range jobs {

		fmt.Println("开始下载 ", job.songname, " ......")

		// create a request
		req, err := http.NewRequest("GET", job.songlink, nil)
		if err != nil {
			return
		}
		req.Close = true
		// send JSON to firebase
		songRes, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("下载文件时出错：", job.songlink)
			return
		}
		songFile, err := os.Create(job.filename)
		if err != nil {
			fmt.Println("创建文件出错：", job.filename)
			return
		}
		written, err := io.Copy(songFile, songRes.Body)
		if err != nil {
			fmt.Println("保存音乐文件时出错：", err)
			songRes.Body.Close()
			os.Remove(job.filename)
			return
		}
		songRes.Body.Close()
		// fmt.Println(job.songname, "下载完成,文件大小：", fmt.Sprintf("%.2f", (float64(written)/(1024*1024))), "MB")

		rets <- fmt.Sprintf("%s 下载完成,文件大小：%.2fMb", job.songname, (float64(written) / (1024 * 1024)))
	}
}

func main() {

	if len(os.Args) <= 1 {
		fmt.Println("请输入网易音乐链接.")
		return
	}
	fmt.Println("fetching msg from ", os.Args[1])

	nurl := strings.Replace(os.Args[1], "#/", "", -1)

	response, err := DownloadString(nurl, nil)
	if err != nil {
		fmt.Println("获取远程URL内容时出错：", err)
		return
	}

	var path string

	if os.IsPathSeparator('\\') {
		path = "\\"
	} else {
		path = "/"
	}
	dir, _ := os.Getwd()

	dir = dir + path + "songs_dir"

	if _, err := os.Stat(dir); err != nil {
		err = os.Mkdir(dir, os.ModePerm)
		if err != nil {
			fmt.Println("创建目录失败：", err)
			return
		}
	}

	jobs := make(chan TDownload, 100)
	rets := make(chan string, 100)
	for i := 0; i < MAX_CROUTINE; i++ {
		go worker(jobs, rets)
	}

	reg := regexp.MustCompile(`<ul class="f-hide">(.*?)</ul>`)

	mm := reg.FindAllString(string(response), -1)

	if len(mm) > 0 {
		reg = regexp.MustCompile(`<li><a .*?>(.*?)</a></li>`)

		contents := mm[0]
		urlli := reg.FindAllSubmatch([]byte(contents), -1)

		for _, item := range urlli {

			//<li><a href="/song?id=469675174">湘楚</a></li>
			// fmt.Printf("item：%s ==== %s\n", string(item[0]), string(item[1]))
			// 查找连续的小写字母
			reg := regexp.MustCompile(`id=[\d]+`)
			idstr := reg.FindAllString(string(item[0]), -1)

			reg = regexp.MustCompile(`[\d]+`)
			id := reg.FindAllString(idstr[0], -1)

			songId := id[0]
			songname := string(item[1])

			urlstr := fmt.Sprintf("%s%s", Neteasymp3url, songId)
			response, err := http.PostForm(urlstr, nil) //client.Do(reqest)
			if err != nil {
				response.Body.Close()
				fmt.Println("获取找到音乐资源失败:", err)
				continue
			}

			res, _ := ioutil.ReadAll(response.Body)
			response.Body.Close()

			var dat map[string]interface{}
			err = json.Unmarshal([]byte(res), &dat)

			if err != nil {
				fmt.Println("反序列化JSON时出错:", err)
				continue
			}
			resData, ok := dat["data"].(map[string]interface{})
			if ok == false {
				fmt.Println("没有找到音乐资源:", resData)
				continue
			}
			songlink, ok := resData["url"].(string)
			if ok == false {
				fmt.Println("没有找到音乐资源:", resData)
				continue
			}

			filename := dir + path + songname + ".mp3"

			jobs <- TDownload{
				filename: filename,
				songname: songname,
				songlink: songlink,
			}
		}

	}
	close(jobs)

	for str := range rets {
		fmt.Println(str)
	}
}

func DownloadString(remoteUrl string, queryValues url.Values) (body []byte, err error) {

	body = nil
	err = nil
	uri, err := url.Parse(remoteUrl)
	if err != nil {
		return
	}
	if queryValues != nil {
		values := uri.Query()
		if values != nil {
			for k, v := range values {
				queryValues[k] = v
			}
		}
		uri.RawQuery = queryValues.Encode()
	}
	reqest, err := http.NewRequest("GET", uri.String(), nil)
	reqest.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	reqest.Header.Add("Accept-Encoding", "gzip, deflate")
	reqest.Header.Add("Accept-Language", "zh-cn,zh;q=0.8,en-us;q=0.5,en;q=0.3")
	reqest.Header.Add("Connection", "keep-alive")
	reqest.Header.Add("Host", uri.Host)
	reqest.Header.Add("Referer", uri.String())
	reqest.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:12.0) Gecko/20100101 Firefox/12.0")

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				deadline := time.Now().Add(30 * time.Second)
				c, err := net.DialTimeout(netw, addr, time.Second*3)
				if err != nil {
					fmt.Println("超时。。", remoteUrl)
					return nil, err
				}
				c.SetDeadline(deadline)
				return c, nil
			},
		},
	}
	response, err := client.Do(reqest)
	defer response.Body.Close()
	if err != nil {
		return
	}

	if response.StatusCode == 200 {
		switch response.Header.Get("Content-Encoding") {
		case "gzip":
			reader, _ := gzip.NewReader(response.Body)
			for {
				buf := make([]byte, 1024)
				n, err := reader.Read(buf)

				if err != nil && err != io.EOF {
					panic(err)
				}

				if n == 0 {
					break
				}
				body = append(body, buf...)
			}
		default:

			body, _ = ioutil.ReadAll(response.Body)

		}
	}
	return
}
