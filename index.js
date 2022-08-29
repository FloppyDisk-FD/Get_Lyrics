import ffmetadata from "ffmetadata";
import inquirer from "inquirer";
import axios from "axios";
import fs from "fs";

const ffmpeg_path = "";
const promptList = [
  {
    type: "input",
    name: "songmid",
    message: "Input the songmid: "
  },
  {
    type: "confirm",
    name: "embed",
    message: "Whether to embed lyrics to a song or not"
  },
  {
    type: "input",
    name: "filename",
    message: "Input the filename: ",
    when: function (answers) { return answers.embed === true; }
  }
];

inquirer.prompt(promptList).then((answers) => {
  var songmid = answers.songmid;
  axios({
    method: "get",
    url: "https://service-47o75c8f-1301683732.sh.apigw.tencentcs.com/release/lyric",
    params: {
      songmid: songmid
    }
  }).then((res) => {
    var lrc = res.data;
    //歌词检测
    switch (answers.embed) {
      case false:
        if (lrc.data["lyric"] === "ºw^~)Þ") {
          console.log("请求失败，songmid可能存在错误！");
        }
        else {
          fs.writeFile("./lyric.txt", lrc.data["lyric"], { flag: "w" }, (err) => {
            if (err) {
              console.log("歌词写入失败，请重试! ");
            }
            else {
              console.log("歌词写入成功! \n歌词在本目录下lyric.txt内");
              return;
            }
          });
        }
        //翻译检测
        if (lrc.data["trans"] === "") {
          console.log("本歌曲不存在翻译！");
        }
        else {
          fs.appendFile("./lyric.txt", lrc.data["trans"], (err) => {
            if (err) {
              console.log("翻译写入失败，请重试!");
            }
            else {
              console.log("翻译写入成功! ");
            }
          });
        }
        break;
      case true:
        var data = {
          lyrics: lrc.data["lyric"] + lrc.data["trans"]
        };
        ffmetadata.setFfmpegPath(ffmpeg_path);
        ffmetadata.write(answers.filename, data, (err) => {
          if (err) {
            console.log("歌词嵌入失败，请重试! ");
          }
          else {
            console.log("歌词嵌入成功! ");
          }
        });
    }
  });
});
