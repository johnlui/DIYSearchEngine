<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta http-equiv="X-UA-Compatible" content="IE=edge">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .title }}</title>
  <link rel="stylesheet" href="http://yeji.zhufaner.com/css/bootstrap.min.css">
  <script src="http://yeji.zhufaner.com/js/vue2.6.11.js"></script>
  <script src="http://yeji.zhufaner.com/js/jquery-2.2.3.min.js"></script>
  <script src="http://yeji.zhufaner.com/js/bootstrap.min.js"></script>
  <script src="http://yeji.zhufaner.com/js/echarts.min.js"></script>
</head>
<body>
  <div id="title">{{ .title }}</div>
  <div id="time">{{ .time }}</div>
  <div id="content">
    <table class="table table-hover">
      <thead>
        <tr>
          <th>Key</th>
          <th>Value</th>
        </tr>
      </thead>
      <tbody>
        {{range $index, $value := .values}}
          <tr>
          {{range $k, $v := $value}}
            <td>
              {{ $k }}
            </td>
            <td>
              {{ $v }}
            </td>
          {{end}}
          </tr>
        {{end}}
      </tbody>
    </table>
  </div>
<style>
* {
  margin: 0;
  padding: 0;
}
#title {
  font-size: 32px;
  text-align: center;
  width: 100%;
  margin-top: 20px;
}
#time {
  font-size: 16px;
  margin: 10px;
  text-align: center;
}
#content {
  width: 600px;
  margin: 20px auto;
}
td, th {
  text-align: center;
}
</style>
</body>
</html>