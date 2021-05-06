package protocol

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// http请求客户端
// 客户端需要维持Session会话
// 并且客户端不允许跳转
type Client struct {
	*http.Client
	UrlManager
	mu      sync.Mutex
	cookies map[string][]*http.Cookie
}

func NewClient(client *http.Client, urlManager UrlManager) *Client {
	return &Client{Client: client, UrlManager: urlManager}
}

// 自动存储cookie
// 设置客户端不自动跳转
func DefaultClient(urlManager UrlManager) *Client {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar: jar,
	}
	return NewClient(client, urlManager)
}

// 抽象Do方法,将所有的有效的cookie存入Client.cookies
// 方便热登陆时获取
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.114 Safari/537.36")
	resp, err := c.Client.Do(req)
	if err != nil {
		return resp, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cookies := resp.Cookies()
	if c.cookies == nil {
		c.cookies = make(map[string][]*http.Cookie)
	}
	c.cookies[resp.Request.URL.String()] = cookies
	return resp, err
}

// 获取当前client的所有的有效的client
func (c *Client) GetCookieMap() map[string][]*http.Cookie {
	return c.cookies
}

// 获取登录的uuid
func (c *Client) GetLoginUUID() (*http.Response, error) {
	path, _ := url.Parse(jsLoginUrl)
	params := url.Values{}
	params.Add("appid", appId)
	params.Add("redirect_uri", c.webWxNewLoginPageUrl)
	params.Add("fun", "new")
	params.Add("lang", "zh_CN")
	params.Add("_", strconv.FormatInt(time.Now().Unix(), 10))
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 获取登录的二维吗
func (c *Client) GetLoginQrcode(uuid string) (*http.Response, error) {
	path := qrcodeUrl + uuid
	return c.Get(path)
}

// 检查是否登录
func (c *Client) CheckLogin(uuid string) (*http.Response, error) {
	path, _ := url.Parse(loginUrl)
	now := time.Now().Unix()
	params := url.Values{}
	params.Add("r", strconv.FormatInt(now/1579, 10))
	params.Add("_", strconv.FormatInt(now, 10))
	params.Add("loginicon", "true")
	params.Add("uuid", uuid)
	params.Add("tip", "0")
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// GetLoginInfo 请求获取LoginInfo
func (c *Client) GetLoginInfo(path string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	req.Header.Add("client-version", uosPatchClientVersion)
	req.Header.Add("extspam", uosPatchExtspam)
	return c.Do(req)
}

// 请求获取初始化信息
func (c *Client) WebInit(request *BaseRequest) (*http.Response, error) {
	path, _ := url.Parse(c.webWxInitUrl)
	params := url.Values{}
	params.Add("_", fmt.Sprintf("%d", time.Now().Unix()))
	path.RawQuery = params.Encode()
	content := struct{ BaseRequest *BaseRequest }{BaseRequest: request}
	body, err := ToBuffer(content)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodPost, path.String(), body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 通知手机已登录
func (c *Client) WebWxStatusNotify(request *BaseRequest, response *WebInitResponse, info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxStatusNotifyUrl)
	params := url.Values{}
	params.Add("lang", "zh_CN")
	params.Add("pass_ticket", info.PassTicket)
	username := response.User.UserName
	content := map[string]interface{}{
		"BaseRequest":  request,
		"ClientMsgId":  time.Now().Unix(),
		"Code":         3,
		"FromUserName": username,
		"ToUserName":   username,
	}
	path.RawQuery = params.Encode()
	buffer, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, path.String(), buffer)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 异步检查是否有新的消息返回
func (c *Client) SyncCheck(info *LoginInfo, response *WebInitResponse) (*http.Response, error) {
	path, _ := url.Parse(c.syncCheckUrl)
	params := url.Values{}
	params.Add("r", strconv.FormatInt(time.Now().Unix(), 10))
	params.Add("skey", info.SKey)
	params.Add("sid", info.WxSid)
	params.Add("uin", strconv.Itoa(info.WxUin))
	params.Add("deviceid", GetRandomDeviceId())
	params.Add("_", strconv.FormatInt(time.Now().Unix(), 10))
	var syncKeyStringSlice []string
	// 将SyncKey里面的元素按照特定的格式拼接起来
	for _, item := range response.SyncKey.List {
		i := fmt.Sprintf("%d_%d", item.Key, item.Val)
		syncKeyStringSlice = append(syncKeyStringSlice, i)
	}
	syncKey := strings.Join(syncKeyStringSlice, "|")
	params.Add("synckey", syncKey)
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 获取联系人信息
func (c *Client) WebWxGetContact(info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxGetContactUrl)
	params := url.Values{}
	params.Add("r", strconv.FormatInt(time.Now().Unix(), 10))
	params.Add("skey", info.SKey)
	params.Add("req", "0")
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 获取联系人详情
func (c *Client) WebWxBatchGetContact(members Members, request *BaseRequest) (*http.Response, error) {
	path, _ := url.Parse(c.webWxBatchGetContactUrl)
	params := url.Values{}
	params.Add("type", "ex")
	params.Add("r", strconv.FormatInt(time.Now().Unix(), 10))
	path.RawQuery = params.Encode()
	list := NewUserDetailItemList(members)
	content := map[string]interface{}{
		"BaseRequest": request,
		"Count":       members.Count(),
		"List":        list,
	}
	body, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, path.String(), body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 获取消息接口
func (c *Client) WebWxSync(request *BaseRequest, response *WebInitResponse, info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxSyncUrl)
	params := url.Values{}
	params.Add("sid", info.WxSid)
	params.Add("skey", info.SKey)
	params.Add("pass_ticket", info.PassTicket)
	path.RawQuery = params.Encode()
	content := map[string]interface{}{
		"BaseRequest": request,
		"SyncKey":     response.SyncKey,
		"rr":          strconv.FormatInt(time.Now().Unix(), 10),
	}
	data, _ := json.Marshal(content)
	body := bytes.NewBuffer(data)
	req, _ := http.NewRequest(http.MethodPost, path.String(), body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 发送消息
func (c *Client) sendMessage(request *BaseRequest, url string, msg *SendMessage) (*http.Response, error) {
	content := map[string]interface{}{
		"BaseRequest": request,
		"Msg":         msg,
		"Scene":       0,
	}
	body, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, url, body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 发送文本消息
func (c *Client) WebWxSendMsg(msg *SendMessage, info *LoginInfo, request *BaseRequest) (*http.Response, error) {
	msg.Type = TextMessage
	path, _ := url.Parse(c.webWxSendMsgUrl)
	params := url.Values{}
	params.Add("lang", "zh_CN")
	params.Add("pass_ticket", info.PassTicket)
	path.RawQuery = params.Encode()
	return c.sendMessage(request, path.String(), msg)
}

// 获取用户的头像
func (c *Client) WebWxGetHeadImg(headImageUrl string) (*http.Response, error) {
	path := c.baseUrl + headImageUrl
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	return c.Do(req)
}

func (c *Client) WebWxUploadMediaByChunk(file *os.File, request *BaseRequest, info *LoginInfo, forUserName, toUserName string) (*http.Response, error) {
	// 获取文件上传的类型
	contentType, err := GetFileContentType(file)
	if err != nil {
		return nil, err
	}

	// 将文件的游标复原到原点
	// 上面获取文件的类型的时候已经读取了512个字节
	if _, err = file.Seek(0, 0); err != nil {
		return nil, err
	}

	buffer := bytes.Buffer{}
	if _, err := buffer.ReadFrom(file); err != nil {
		return nil, err
	}
	data := buffer.Bytes()
	fileMd5 := fmt.Sprintf("%x", md5.Sum(data))

	sate, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// 获取文件的类型
	mediaType := getMessageType(sate.Name())

	path, _ := url.Parse(c.webWxUpLoadMediaUrl)
	params := url.Values{}
	params.Add("f", "json")

	path.RawQuery = params.Encode()

	cookies := c.Jar.Cookies(path)
	webWxDataTicket := getWebWxDataTicket(cookies)

	uploadMediaRequest := map[string]interface{}{
		"UploadType":    2,
		"BaseRequest":   request,
		"ClientMediaId": time.Now().Unix() * 1e4,
		"TotalLen":      sate.Size(),
		"StartPos":      0,
		"DataLen":       sate.Size(),
		"MediaType":     4,
		"FromUserName":  forUserName,
		"ToUserName":    toUserName,
		"FileMd5":       fileMd5,
	}

	uploadMediaRequestByte, err := json.Marshal(uploadMediaRequest)
	if err != nil {
		return nil, err
	}

	var chunks int64

	if sate.Size() > chunkSize {
		chunks = sate.Size() / chunkSize
		if chunks*chunkSize < sate.Size() {
			chunks++
		}
	} else {
		chunks = 1
	}

	var resp *http.Response

	content := map[string]string{
		"id":                "WU_FILE_0",
		"name":              file.Name(),
		"type":              contentType,
		"lastModifiedDate":  sate.ModTime().Format(TimeFormat),
		"size":              strconv.FormatInt(sate.Size(), 10),
		"mediatype":         mediaType,
		"webwx_data_ticket": webWxDataTicket,
		"pass_ticket":       info.PassTicket,
	}

	if chunks > 1 {
		content["chunks"] = strconv.FormatInt(chunks, 10)
	}
	// 分块上传
	for chunk := 0; int64(chunk) < chunks; chunk++ {

		var isLastTime bool
		if int64(chunk)+1 == chunks {
			isLastTime = true
		}

		if chunks > 1 {
			content["chunk"] = strconv.Itoa(chunk)
		}

		var formBuffer bytes.Buffer

		writer := multipart.NewWriter(&formBuffer)
		if err = writer.WriteField("uploadmediarequest", string(uploadMediaRequestByte)); err != nil {
			return nil, err
		}

		for k, v := range content {
			if err := writer.WriteField(k, v); err != nil {
				return nil, err
			}
		}

		if w, err := writer.CreateFormFile("filename", file.Name()); err != nil {
			return nil, err
		} else {
			var chunkData []byte
			// 判断是不是最后一次
			if !isLastTime {
				chunkData = data[int64(chunk)*chunkSize : (int64(chunk)+1)*chunkSize]
			} else {
				chunkData = data[int64(chunk)*chunkSize:]
			}
			if _, err = w.Write(chunkData); err != nil {
				return nil, err
			}
		}
		ct := writer.FormDataContentType()
		if err = writer.Close(); err != nil {
			return nil, err
		}
		req, _ := http.NewRequest(http.MethodPost, path.String(), &formBuffer)
		req.Header.Set("Content-Type", ct)
		// 发送数据
		resp, err = c.Do(req)

		// 如果不是最后一次, 解析有没有错误
		if !isLastTime {
			returnResp := NewReturnResponse(resp, err)
			if err := parseBaseResponseError(returnResp); err != nil {
				return nil, err
			}
		}
	}
	// 将最后一次携带文件信息的response返回
	return resp, err
}

// 发送图片
// 这个接口依赖上传文件的接口
// 发送的图片必须是已经成功上传的图片
func (c *Client) WebWxSendMsgImg(msg *SendMessage, request *BaseRequest, info *LoginInfo) (*http.Response, error) {
	msg.Type = ImageMessage
	path, _ := url.Parse(c.webWxSendMsgImgUrl)
	params := url.Values{}
	params.Add("fun", "async")
	params.Add("f", "json")
	params.Add("lang", "zh_CN")
	params.Add("pass_ticket", info.PassTicket)
	path.RawQuery = params.Encode()
	return c.sendMessage(request, path.String(), msg)
}

// 发送文件信息
func (c *Client) WebWxSendAppMsg(msg *SendMessage, request *BaseRequest) (*http.Response, error) {
	msg.Type = AppMessage
	path, _ := url.Parse(c.webWxSendAppMsgUrl)
	params := url.Values{}
	params.Add("fun", "async")
	params.Add("f", "json")
	path.RawQuery = params.Encode()
	return c.sendMessage(request, path.String(), msg)
}

// 用户重命名接口
func (c *Client) WebWxOplog(request *BaseRequest, remarkName, userName string) (*http.Response, error) {
	path, _ := url.Parse(c.webWxOplogUrl)
	params := url.Values{}
	params.Add("lang", "zh_CN")
	path.RawQuery = params.Encode()
	content := map[string]interface{}{
		"BaseRequest": request,
		"CmdId":       2,
		"RemarkName":  remarkName,
		"UserName":    userName,
	}
	body, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, path.String(), body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 添加用户为好友接口
func (c *Client) WebWxVerifyUser(storage *Storage, info RecommendInfo, verifyContent string) (*http.Response, error) {
	loginInfo := storage.LoginInfo
	path, _ := url.Parse(c.webWxVerifyUserUrl)
	params := url.Values{}
	params.Add("r", strconv.FormatInt(time.Now().Unix(), 10))
	params.Add("lang", "zh_CN")
	params.Add("pass_ticket", loginInfo.PassTicket)
	path.RawQuery = params.Encode()
	content := map[string]interface{}{
		"BaseRequest":    storage.Request,
		"Opcode":         3,
		"SceneList":      [1]int{33},
		"SceneListCount": 1,
		"VerifyContent":  verifyContent,
		"VerifyUserList": []interface{}{map[string]string{
			"Value":            info.UserName,
			"VerifyUserTicket": info.Ticket,
		}},
		"VerifyUserListSize": 1,
		"skey":               loginInfo.SKey,
	}
	body, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, path.String(), body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}

// 获取图片消息的图片响应
func (c *Client) WebWxGetMsgImg(msg *Message, info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxGetMsgImgUrl)
	params := url.Values{}
	params.Add("MsgID", msg.MsgId)
	params.Add("skey", info.SKey)
	params.Add("type", "slave")
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 获取语音消息的语音响应
func (c *Client) WebWxGetVoice(msg *Message, info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxGetVoiceUrl)
	params := url.Values{}
	params.Add("msgid", msg.MsgId)
	params.Add("skey", info.SKey)
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 获取视频消息的视频响应
func (c *Client) WebWxGetVideo(msg *Message, info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxGetVideoUrl)
	params := url.Values{}
	params.Add("msgid", msg.MsgId)
	params.Add("skey", info.SKey)
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 获取文件消息的文件响应
func (c *Client) WebWxGetMedia(msg *Message, info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxGetMediaUrl)
	params := url.Values{}
	params.Add("sender", msg.FromUserName)
	params.Add("mediaid", msg.MediaId)
	params.Add("encryfilename", msg.EncryFileName)
	params.Add("fromuser", fmt.Sprintf("%d", info.WxUin))
	params.Add("pass_ticket", info.PassTicket)
	params.Add("webwx_data_ticket", getWebWxDataTicket(c.Jar.Cookies(path)))
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 用户退出
func (c *Client) Logout(info *LoginInfo) (*http.Response, error) {
	path, _ := url.Parse(c.webWxLogoutUrl)
	params := url.Values{}
	params.Add("redirect", "1")
	params.Add("type", "1")
	params.Add("skey", info.SKey)
	path.RawQuery = params.Encode()
	req, _ := http.NewRequest(http.MethodGet, path.String(), nil)
	return c.Do(req)
}

// 添加用户进群聊
func (c *Client) AddMemberIntoChatRoom(req *BaseRequest, info *LoginInfo, group *Group, friends ...*Friend) (*http.Response, error) {
	path, _ := url.Parse(c.webWxUpdateChatRoomUrl)
	params := url.Values{}
	params.Add("fun", "addmember")
	params.Add("pass_ticket", info.PassTicket)
	params.Add("lang", "zh_CN")
	path.RawQuery = params.Encode()
	addMemberList := make([]string, 0)
	for _, friend := range friends {
		addMemberList = append(addMemberList, friend.UserName)
	}
	content := map[string]interface{}{
		"ChatRoomName":  group.UserName,
		"BaseRequest":   req,
		"AddMemberList": strings.Join(addMemberList, ","),
	}
	buffer, _ := ToBuffer(content)
	requ, _ := http.NewRequest(http.MethodPost, path.String(), buffer)
	requ.Header.Set("Content-Type", jsonContentType)
	return c.Do(requ)
}

// 从群聊中移除用户
func (c *Client) RemoveMemberFromChatRoom(req *BaseRequest, info *LoginInfo, group *Group, friends ...*User) (*http.Response, error) {
	path, _ := url.Parse(c.webWxUpdateChatRoomUrl)
	params := url.Values{}
	params.Add("fun", "delmember")
	params.Add("lang", "zh_CN")
	params.Add("pass_ticket", info.PassTicket)
	delMemberList := make([]string, 0)
	for _, friend := range friends {
		delMemberList = append(delMemberList, friend.UserName)
	}
	content := map[string]interface{}{
		"ChatRoomName":  group.UserName,
		"BaseRequest":   req,
		"DelMemberList": strings.Join(delMemberList, ","),
	}
	buffer, _ := ToBuffer(content)
	requ, _ := http.NewRequest(http.MethodPost, path.String(), buffer)
	requ.Header.Set("Content-Type", jsonContentType)
	return c.Do(requ)
}

// 撤回消息
func (c *Client) WebWxRevokeMsg(msg *SentMessage, request *BaseRequest) (*http.Response, error) {
	content := map[string]interface{}{
		"BaseRequest": request,
		"ClientMsgId": msg.ClientMsgId,
		"SvrMsgId":    msg.MsgId,
		"ToUserName":  msg.ToUserName,
	}
	buffer, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, c.webWxRevokeMsg, buffer)
	req.Header.Set("Content-Type", jsonContentType)
	return c.Do(req)
}

// 校验上传文件
func (c *Client) webWxCheckUpload(stat os.FileInfo, request *BaseRequest, fileMd5, fromUserName, toUserName string) (*http.Response, error) {
	path, _ := url.Parse(c.webWxCheckUploadUrl)
	content := map[string]interface{}{
		"BaseRequest":  request,
		"FileMd5":      fileMd5,
		"FileName":     stat.Name(),
		"FileSize":     stat.Size(),
		"FileType":     7,
		"FromUserName": fromUserName,
		"ToUserName":   toUserName,
	}
	body, _ := ToBuffer(content)
	req, _ := http.NewRequest(http.MethodPost, path.String(), body)
	req.Header.Add("Content-Type", jsonContentType)
	return c.Do(req)
}
