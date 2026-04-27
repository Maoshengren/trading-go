APP_NAME := trading-go
OUTPUT_DIR := output
BIN := $(OUTPUT_DIR)/$(APP_NAME)
PID_FILE := $(OUTPUT_DIR)/$(APP_NAME).pid
RUN_LOG := $(OUTPUT_DIR)/$(APP_NAME).stdout.log
GO ?= go
GOCACHE ?= $(CURDIR)/.gocache

.PHONY: help build run start stop restart status logs test clean

help:
	@echo "Targets:"
	@echo "  make build    Compile $(APP_NAME) into $(OUTPUT_DIR)/"
	@echo "  make run      Build and run in foreground"
	@echo "  make start    Build and run in background"
	@echo "  make stop     Stop background process"
	@echo "  make restart  Stop then start background process"
	@echo "  make status   Show background process status"
	@echo "  make logs     Tail background stdout/stderr log"
	@echo "  make test     Run all tests"
	@echo "  make clean    Remove build output"

build:
	@mkdir -p $(OUTPUT_DIR)
	GOCACHE=$(GOCACHE) $(GO) build -o $(BIN) .
	@echo "Built $(BIN)"

run: build
	$(BIN)

start: build
	@mkdir -p $(OUTPUT_DIR)
	@if [ -f "$(PID_FILE)" ] && kill -0 "$$(cat $(PID_FILE))" 2>/dev/null; then \
		echo "$(APP_NAME) is already running with pid $$(cat $(PID_FILE))"; \
		exit 1; \
	fi
	@nohup $(BIN) > "$(RUN_LOG)" 2>&1 & echo $$! > "$(PID_FILE)"
	@echo "$(APP_NAME) started with pid $$(cat $(PID_FILE))"
	@echo "Log: $(RUN_LOG)"

stop:
	@if [ ! -f "$(PID_FILE)" ]; then \
		echo "$(APP_NAME) is not running: missing $(PID_FILE)"; \
		exit 0; \
	fi
	@pid="$$(cat $(PID_FILE))"; \
	if kill -0 "$$pid" 2>/dev/null; then \
		kill "$$pid"; \
		echo "Stopping $(APP_NAME) pid $$pid"; \
		for i in 1 2 3 4 5; do \
			if kill -0 "$$pid" 2>/dev/null; then sleep 1; else break; fi; \
		done; \
		if kill -0 "$$pid" 2>/dev/null; then \
			echo "$(APP_NAME) pid $$pid is still running; use kill $$pid if you need to force stop"; \
			exit 1; \
		fi; \
	else \
		echo "$(APP_NAME) pid $$pid is not running"; \
	fi; \
	rm -f "$(PID_FILE)"

restart: stop start

status:
	@if [ -f "$(PID_FILE)" ] && kill -0 "$$(cat $(PID_FILE))" 2>/dev/null; then \
		echo "$(APP_NAME) is running with pid $$(cat $(PID_FILE))"; \
	else \
		echo "$(APP_NAME) is not running"; \
	fi

logs:
	@mkdir -p $(OUTPUT_DIR)
	@touch "$(RUN_LOG)"
	tail -f "$(RUN_LOG)"

test:
	GOCACHE=$(GOCACHE) $(GO) test ./...

clean:
	rm -rf "$(OUTPUT_DIR)"
