package main

import (
    "net/http"
)

func serveChatPage(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/html")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(chat_page))
}

const chat_page = `<html>
    <head>
        <title> Dummy chat server </title>
        <meta charset="utf-8" name="viewport" />

        <style>
            body {
                padding-left: 10%;
                padding-right: 10%;
                font-size: large;
            }
            div {
                display: flex;
                flex-direction: row;
                align-items: baseline;
                margin-bottom: 0.25em;
            }
            label {
                font-size: large;
            }
            input.text {
                margin-left: 1em;
                margin-right: 1em;
                height: 2em;
                font-size: large;
            }
            input.button {
                height: 2em;
                font-size: large;
            }
            input.textbox {
                width: 90%;
                margin-right: 0.25em;
                margin-top: 0.25em;
                height: 2em;
                font-size: large;
            }
            div.textbox {
                display: block;
                width: 95%;
                height: 75%;
                margin-top: 0.25em;
                overflow-y: scroll;
                border: solid;
                padding: 1em;
            }
        </style>

        <script>
            let ws = null;
            let channel = '';
            let username = '';

            let appendMsg = function(msg) {
                let chat = document.getElementById('chat');
                chat.innerHTML += msg;
                chat.scrollTo(0, chat.scrollHeight);
            }

            let wsRecv = function(e) {
                let msg = e.data;
                appendMsg('<p> ' + msg + ' </p>');
            }

            let wsClose = function(e) {
                appendMsg('<p> Connection to the channel was closed! </p>');
                ws = null;
            }

            let wsErr = function(e) {
                appendMsg('<p> Failed to receive a message from the channel! </p>');
                ws = null;
            }

            let connect = function() {
                let cfield = document.getElementById('channel');
                let ufield = document.getElementById('username');

                channel = cfield.value;
                username = ufield.value;

                if (ws != null) {
                    ws.close()
                    ws = null;
                }

                let xhr = new XMLHttpRequest();
                xhr.open("POST", '/new_token/' + channel + '/' + username, true);
                xhr.addEventListener("loadend", function (e) {
                    // 200 == OK
                    if (e.target.status == 200) {
                        tk = e.target.response;

                        document.cookie = 'X-ChatToken=' + tk

                        proto = 'ws'
                        if (window.location.protocol.startsWith('https')) {
                            proto = 'wss'
                        }

                        ws = new WebSocket(proto + '://' + window.location.host + '/chat')
                        ws.addEventListener('message', wsRecv)
                        ws.addEventListener('close', wsClose)

                        appendMsg('<p> Now talking on ' + channel + '! </p>');

                        document.cookie = 'X-ChatToken=XXXXXXXX'
                    }
                    else {
                        appendMsg('<p> Error: ' + e.target.response + '! </p>');
                    }
                });
                xhr.addEventListener("error", function (e) {
                    appendMsg('<p> Error: ' + e.target + '! </p>');
                });
                xhr.addEventListener("loadstart", _ignore);
                xhr.addEventListener("load", _ignore);
                xhr.addEventListener("progress", _ignore);

                xhr.send({});
            }

            let send = function() {
                let mfield = document.getElementById('message');

                let msg = mfield.value;
                if (msg == '') {
                    return;
                }

                ws.send(msg);
                appendMsg('<p> <small> just sent: ' + username + ' - ' + msg + ' </small> </p>');

                mfield.value = '';
            }

            let _ignore = function(e) {}

            let create_channel = function() {
                let cfield = document.getElementById('channel');
                channel = cfield.value;

                let xhr = new XMLHttpRequest();
                xhr.open("POST", '/new_channel/' + channel, true);
                xhr.addEventListener("loadend", function (e) {
                    // 204 == No Content
                    if (e.target.status == 204) {
                        appendMsg('<p> Channel "' + channel + '" created successfully! </p>');
                    }
                    else {
                        appendMsg('<p> Error: ' + e.target.response + '! </p>');
                    }
                });
                xhr.addEventListener("error", function (e) {
                    appendMsg('<p> Error: ' + e.target + '! </p>');
                });
                xhr.addEventListener("loadstart", _ignore);
                xhr.addEventListener("load", _ignore);
                xhr.addEventListener("progress", _ignore);

                xhr.send({});
            }

            let on_boot = function (e) {
                let mfield = document.getElementById('message');
                mfield.addEventListener('keyup', function (e) {
                    if (event.key == 'Enter') {
                        send();
                    }
                });
            }
            document.addEventListener('DOMContentLoaded', on_boot);
        </script>
    </head>

    <body>
        <div>
            <label for='channel'> Channel: </label>
            <input class='text' type='text' id='channel' name='channel'>
            <input class='button' onclick="create_channel();" type="button" value="Create Channel">
        </div>
        <div>
            <label for='username'> Username: </label>
            <input class='text' type='text' id='username' name='username'>
            <input class='button' onclick="connect();" type="button" value="Connect">
        </div>

        <div class='textbox' id='chat'> </div>

        <div>
            <input class='textbox' type='text' id='message' name='message'>
            <input class='button' onclick="send();" type="button" value="Send">
        </div>
    </body>
</html>`
