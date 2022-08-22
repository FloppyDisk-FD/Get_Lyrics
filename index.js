import inquirer from "inquirer";
import axios from "axios";
import fs from "fs";

const prompt = [
  {
    type: "input",
    name: "songmid",
    message: "Input the songmid: "
  }
];

inquirer.prompt(prompt).then((answers) => {
  var songmid = answers.songmid;

  axios({
    method: "get",
    url: "http://127.0.0.1:3300/lyric",
    params: {
      songmid: songmid
    }
  }).then((res) => {
    var lrc = res.data;
    //歌词检测
    if(lrc.data["lyric"]=="ºw^~)Þ"){
      console.log("请求失败，songmid可能存在错误！");
    }
    else{
      fs.writeFile("./lyric.txt", lrc.data["lyric"], { flag: 'w' }, (err) => {
        if(err){
          console.log("歌词写入失败，请重试！");
        }
        else{
          console.log("歌词写入成功!\n歌词在本目录下lyric.txt内");
          return
        }
      })
    }
    //翻译检测
    if(lrc.data["trans"]==""){
      console.log("本歌曲不存在翻译！");
    }
    else{
      fs.appendFile("./lyric.txt", lrc.data["trans"],(err) => {
        if(err){
          console.log("翻译写入失败，请重试！");
        }
        else{
          console.log("翻译写入成功！");
        }   
      })
    }
  });
});
