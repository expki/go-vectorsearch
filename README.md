# VectorSearch Project

## Overview
This project uses Ollama to generate vector embeddings for uploaded documents and provides search functions that allow users to query previous uploads. Documents can be stored in either SQLite or PostgreSQL databases.

## Features
- **Vector Embeddings**: Utilizes Ollama to create vector embeddings from user-uploaded documents.
- **Search Functionality**: Enables users to perform searches through previously uploaded documents using the generated embeddings.
- **Database Storage**: Supports storing documents in either SQLite for simplicity and local development, or PostgreSQL for more robust production environments.
- **Swagger Documentation**: Comes with Swagger documentation for easy API interaction and testing.

## Operations
Download a release.
Running the executable will produce a `config.json` file in the current directory. Modify this file to configure your database settings and other parameters.

### Configuration
The `config.json` file contains all necessary configuration for the application, including:
- Database type (SQLite or PostgreSQL)
- Connection strings for each database type
- Ollama server URL
Not all configuration is required.
```json
{
    "server": {
        "http_address": ":7500",
        "https_address": ":7501"
    },
    "tls": {
        "dns": ["computer001.localdomain"],
        "ip": ["192.168.1.100"],
        "certificates": [
            {
                "cert_path": "/etc/ssl/certs/server.pem",
                "key_path": "/etc/ssl/private/server.pem"
            }
        ]
    },
    "database": {
        "sqlite": "./vectors.db",
        "postgres": ["host=localhost user=vectorsearch password=1234 dbname=vectordb port=9920 sslmode=disable"],
        "postgres_readonly": ["host=localhost user=vectorsearch password=1234 dbname=vectordb port=9920 sslmode=disable"]
    },
    "ollama": {
        "url": "https://ollama.vdh.dev",
        "embed": "nomic-embed-text",
        "generate": "llama3.2",
        "chat": "llama3.2",
        "token": "bearer-auth-token-1234"
    }
}

```

## Development
To get started with this project, follow these steps:

### Prerequisites
- Debian Linux or WSL (Ubuntu recommeded)
- Go 1.24 or later
- Ollama Server

### Build
```bash
git clone https://github.com/your-repo/go-vectorsearch.git
cd go-vectorsearch
./build.sh
```
### Run
```bash
git clone https://github.com/your-repo/go-vectorsearch.git
cd go-vectorsearch
./run.sh
```
