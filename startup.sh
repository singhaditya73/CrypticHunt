#!/bin/bash

# Install templ
go install github.com/a-h/templ/cmd/templ@v0.2.778

# Generate templates
~/go/bin/templ generate

# Install npm dependencies and build CSS
npm install
npx tailwindcss -i ./assets/app.css -o ./public/app.css

# Build the Go application
go build -o app ./app/

# Run the application
./app
