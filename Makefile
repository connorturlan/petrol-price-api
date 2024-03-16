.PHONY: build

build:
	sam build
	
deploy: build
	sam deploy --no-confirm-changeset --no-fail-on-empty-changeset

local: build up
	sam local start-api -n env.json --warm-containers eager --docker-network lambda-local

up:
	docker-compose up -d
down:
	docker-compose down
	
sync: build
	sam sync --stack-name unisa-court-booking
watch: build
	sam sync --stack-name unisa-court-booking --watch
