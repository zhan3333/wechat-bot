package main

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	"github.com/silenceper/wechat/v2/officialaccount"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/material"
	"github.com/silenceper/wechat/v2/officialaccount/message"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"wechat-bot/config"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ocr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ocr/v20181119"
)

import "fmt"

var wc *wechat.Wechat
var oa *officialaccount.OfficialAccount

func main() {
	app := gin.Default()

	//使用memcache保存access_token，也可选择redis或自定义cache
	wc = wechat.NewWechat()
	memory := cache.NewMemory()
	cfg := &offConfig.Config{
		AppID:          config.Default.WeChat.AppID,
		AppSecret:      config.Default.WeChat.AppSecret,
		Token:          config.Default.WeChat.Token,
		EncodingAESKey: config.Default.WeChat.EncodingAESKey,
		Cache:          memory,
	}
	oa = wc.GetOfficialAccount(cfg)

	app.GET("/debug", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"msg": "wechat-bot run ok",
		})
	})
	app.Any("/api/wechat", func(c *gin.Context) {
		fmt.Println("api wechat receive")
		// 传入request和responseWriter
		server := oa.GetServer(c.Request, c.Writer)
		//设置接收消息的处理方法
		server.SetMessageHandler(wechatMsgHandel)

		//处理消息接收以及回复
		err := server.Serve()
		if err != nil {
			fmt.Println(err)
			return
		}
		//发送回复的消息
		if err := server.Send(); err != nil {
			fmt.Println(err)
		}
	})

	if err := app.Run(config.Default.Addr); err != nil {
		log.Fatalf("listen: %s\n", err)
	}
}

type funcMapType map[message.MsgType]func(msg *message.MixMessage) *message.Reply

var msgFuncMap = funcMapType{
	message.MsgTypeText:  receiveText,
	message.MsgTypeImage: receiveImage,
	message.MsgTypeEvent: receiveEvent,
}

func receiveEvent(msg *message.MixMessage) *message.Reply {
	if msg.Event == message.EventSubscribe {
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText(defaultMsg),
		}
	}
	return nil
}

// 接收微信消息并做出响应
func wechatMsgHandel(msg *message.MixMessage) *message.Reply {
	if f, ok := msgFuncMap[msg.MsgType]; ok {
		return f(msg)
	} else {
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText(fmt.Sprintf("不支持处理的消息类型: %s", msg.MsgType)),
		}
	}
}

var textFuncMap = map[string]func(msg string) *message.Reply{
	"动物": receiveTextAnimal,
}

var defaultMsg = `这是默认的回复: 

- 发送图片可以识别图片中的文本
`

func receiveText(msg *message.MixMessage) *message.Reply {
	if f, ok := textFuncMap[msg.Content]; ok {
		return f(msg.Content)
	} else {
		return &message.Reply{MsgType: message.MsgTypeText, MsgData: message.NewText(defaultMsg)}
	}
}

func receiveTextAnimal(_ string) *message.Reply {
	cli := http.Client{Timeout: 3 * time.Second}
	resp, err := cli.Get("https://zoo-animal-api.herokuapp.com/animals/rand")
	if err != nil {
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText("请求动物失败: " + err.Error()),
		}
	}
	defer func() { _ = resp.Body.Close() }()
	body := struct {
		Name      string `json:"name"`
		ImageLink string `json:"image_link"`
	}{}
	b, _ := ioutil.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &body)
	if body.ImageLink == "" {
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText("解析动物图片链接失败"),
		}
	}
	// 上传到媒体库
	mediaID, err := downloadImgUploadMedia(body.ImageLink)
	if err != nil {
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText(err.Error()),
		}
	}

	// 响应媒体图片
	return &message.Reply{MsgType: message.MsgTypeImage, MsgData: message.NewImage(mediaID)}
}

func downloadImgUploadMedia(imgURL string) (string, error) {
	fmt.Println("download image: ", imgURL)
	cli := http.Client{Timeout: 3 * time.Second}
	resp2, err := cli.Get(imgURL)
	if err != nil {
		return "", fmt.Errorf("下载图片失败: %w", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	b2, _ := ioutil.ReadAll(resp2.Body)
	tmp, err := ioutil.TempFile(os.TempDir(), "animal-")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	_, err = tmp.Write(b2)
	if err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	m := oa.GetMaterial()
	mediaID, mediaURL, err := m.AddMaterial(material.MediaTypeImage, tmp.Name())
	fmt.Println("media: ", mediaID, mediaURL, err)
	if err != nil {
		return "", fmt.Errorf("上传到媒体库失败: %w", err)
	}
	return mediaID, nil
}

// 收到图片时，将调用腾讯云图片识别 api，识别图片中文字
func receiveImage(msg *message.MixMessage) *message.Reply {
	imgText, err := func() (string, error) {
		credential := common.NewCredential(
			config.Default.Tencent.SecretID,
			config.Default.Tencent.SecretKey,
		)
		cpf := profile.NewClientProfile()
		cpf.HttpProfile.Endpoint = "ocr.tencentcloudapi.com"
		client, _ := ocr.NewClient(credential, "ap-beijing", cpf)

		request := ocr.NewGeneralBasicOCRRequest()
		picURL := msg.PicURL
		fmt.Println("picURL: ", picURL)
		request.ImageUrl = &picURL

		response, err := client.GeneralBasicOCR(request)
		if _, ok := err.(*errors.TencentCloudSDKError); ok {
			fmt.Printf("An API error has returned: %s", err)
			return "", fmt.Errorf("an API error has returned: %w", err)
		}
		if err != nil {
			return "", fmt.Errorf("request api error: %w", err)
		}
		bodyText := "检测到以下文本:\n\n"
		for _, t := range response.Response.TextDetections {
			bodyText += *t.DetectedText + "\n"
		}
		return bodyText, nil
	}()
	text := message.NewText("")
	if err != nil {
		text.Content = message.CDATA(err.Error())
	} else {
		text.Content = message.CDATA(imgText)
	}
	return &message.Reply{MsgType: message.MsgTypeText, MsgData: text}
}
