# Get_Lyrics
一个简易获取QQ音乐歌词的工具

## 使用
### - 获取歌词
请预先安装Node环境
#### 1. 安装依赖

```
pnpm install
```
或
```
npm install
```
#### 2.运行
```
node index.js
```
#### 3.获取 songmid
在QQ音乐APP或客户端复制歌曲链接，在浏览器中打开，会得到如`https://y.qq.com/n/ryqq/songDetail/********`风格链接，例如：
```
https://y.qq.com/n/ryqq/songDetail/0017K7gL4WYnw2
```
那么`0017K7gL4WYnw2`就是我们需要的songmid
#### 4.输入songmid
运行`index.js`后，在命令行中输入获取到的songmid，输入后歌词将存于本目录下`lyric.txt`文件中
### - 嵌入歌词
####方法1: 使用工具自带的嵌入功能
前提条件: 下载 [ffmpeg](https://ffmpeg.org/ "ffmpeg") ，放入指定目录
##### 1. 编辑脚本
编辑`index.js`文件
```
const ffmpeg_path = "你的ffmpeg所在目录";
//例如
const ffmpeg_path = "./ffmpeg/ffmpeg.exe";
```
##### 2.输入歌曲文件名
在提示的问题中输入歌曲文件名，例如歌曲文件名为`test.flac`，那么就输入***test.flac***
####方法2: 使用音乐标签APP

## TODO
- [x] 将歌词直接内嵌歌曲
- [ ] 网页操作
- [ ] 增加网易云音乐源
- [ ] 完整的标签修改
- [ ] 安卓APP
## 其他
本工具使用到的API为 [https://github.com/jsososo/QQMusicApi](https://github.com/jsososo/QQMusicApi)
