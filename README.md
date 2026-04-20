# URL Shortener Service

A lightweight URL shortening service written in Go with PostgreSQL for storage and Redis for caching.

## Features

- Shorten long URLs to 8-character codes
- Redirect to original URLs
- Click counter for each link
- Redis caching for fast redirects
- Asynchronous click counting
- Docker Compose setup

## Tech Stack

- Go 1.22
- PostgreSQL 16
- Redis 7
- pgx PostgreSQL driver
- go-redis client
- Docker

## Requirements

- Docker and Docker Compose
- Go 1.22 or higher (for local development)

## Installation

Clone the repository:

```bash
git clone https://github.com/FEDESSSS/url-shortener.git
cd url-shortener
