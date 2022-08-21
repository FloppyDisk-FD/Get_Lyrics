import inquirer from "inquirer";
import axios from "axios";
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
    console.log(res.data);
  });
});
