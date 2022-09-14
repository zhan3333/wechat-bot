upload:
	rm build/wechat-bot build/config.json
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/wechat-bot main.go
	cp config.json build/config.json
	scp -r build/* zhan@t:/home/zhan/Application/wechat-bot/
	ssh zhan@t "cd ~/Application && docker-compose up --build --no-deps -d wechat-bot"