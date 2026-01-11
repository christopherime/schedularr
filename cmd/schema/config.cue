package config

tunarr: {
	url: string | *"http://localhost:8000"
	api_key: string | *""
}

log: {
	level: "debug" | "info" | "warn" | "error" | *"info"
	format: "text" | "json" | *"text"
}

scheduler_file: string | *"scheduler.yaml"
