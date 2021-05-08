package protocol

import (
	"encoding/json"
	"log"
	"net/http"
)

// Storage 身份信息, 维持整个登陆的Session会话
type Storage struct {
	LoginInfo *LoginInfo
	Request   *BaseRequest
	Response  *WebInitResponse
}

// HotReloadStorage 热登陆存储接口
type HotReloadStorage interface {
	GetCookie() map[string][]*http.Cookie                                            // 获取client.cookie
	GetBaseRequest() *BaseRequest                                                    // 获取BaseRequest
	GetLoginInfo() *LoginInfo                                                        // 获取LoginInfo
	Dump(cookies map[string][]*http.Cookie, req *BaseRequest, info *LoginInfo) error // 实现该方法, 将必要信息进行序列化
	Load() error                                                                     // 实现该方法, 将存储媒介的内容反序列化
}

// JsonFileHotReloadStorage 实现HotReloadStorage接口
// 默认以json文件的形式存储
type JsonFileHotReloadStorage struct {
	Cookie   map[string][]*http.Cookie
	Req      *BaseRequest
	Info     *LoginInfo
	filename string
}

// Dump 将信息写入json文件
//func (f *JsonFileHotReloadStorage) Dump(cookies map[string][]*http.Cookie, req *BaseRequest, info *LoginInfo) error {
//
//	file, err := os.OpenFile(f.filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
//
//	if err != nil {
//		return err
//	}
//
//	defer file.Close()
//
//	f.Cookie = cookies
//	f.Req = req
//	f.Info = info
//
//	data, err := json.Marshal(f)
//	if err != nil {
//		return err
//	}
//	_, err = file.Write(data)
//	return err
//}

// Dump 将信息写入Redis
func (f *JsonFileHotReloadStorage) Dump(cookies map[string][]*http.Cookie, req *BaseRequest, info *LoginInfo) error {
	f.Cookie = cookies
	f.Req = req
	f.Info = info

	data, err := json.Marshal(f)
	if err != nil {
		log.Println("序列化微信热登录信息失败：", err.Error())
		return err
	}
	// 保存信息到Redis
	err = set(f.filename, string(data))
	if err != nil {
		log.Println("保存微信热登录信息失败：", err.Error())
		return err
	}
	return nil
}

// Load 从文件中读取信息
//func (f *JsonFileHotReloadStorage) Load() error {
//	file, err := os.Open(f.filename)
//
//	if err != nil {
//		return err
//	}
//	defer file.Close()
//	var buffer bytes.Buffer
//	if _, err := buffer.ReadFrom(file); err != nil {
//		return err
//	}
//	err = json.Unmarshal(buffer.Bytes(), f)
//	return err
//}

// Load 从Redis读取信息
func (f *JsonFileHotReloadStorage) Load() error {
	// 从Redis获取热登录数据
	data, err := get(f.filename)
	if err != nil {
		log.Println("读取微信热登录数据失败：", err.Error())
		return err
	}
	// 反序列化热登录数据
	err = json.Unmarshal([]byte(data), f)
	return err
}

// GetCookie 获取Cookie
func (f *JsonFileHotReloadStorage) GetCookie() map[string][]*http.Cookie {
	return f.Cookie
}

// GetBaseRequest 获取BaseRequest
func (f *JsonFileHotReloadStorage) GetBaseRequest() *BaseRequest {
	return f.Req
}

// GetLoginInfo 获取登录信息
func (f *JsonFileHotReloadStorage) GetLoginInfo() *LoginInfo {
	return f.Info
}

// NewJsonFileHotReloadStorage 新建一个JsonStorage对象
func NewJsonFileHotReloadStorage(filename string) *JsonFileHotReloadStorage {
	return &JsonFileHotReloadStorage{filename: filename}
}
