#!/bin/bash
# Script to run the Parks App test suite

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo "=========================================="
echo "Parks App - Click Tracking Test Suite"
echo "=========================================="
echo ""

# Check if Python is installed
if ! command -v python3 &> /dev/null; then
    echo "❌ Error: Python 3 is not installed"
    exit 1
fi

# Check if pip is installed
if ! command -v pip3 &> /dev/null; then
    echo "❌ Error: pip3 is not installed"
    exit 1
fi

# Install dependencies
echo "📦 Installing test dependencies..."
pip3 install -q -r requirements.txt

echo "✅ Dependencies installed"
echo ""

# Check if URL is provided as argument or environment variable
if [ -n "$1" ]; then
    export PARKS_APP_URL="$1"
    echo "🎯 Testing against: $PARKS_APP_URL"
elif [ -n "$PARKS_APP_URL" ]; then
    echo "🎯 Testing against: $PARKS_APP_URL (from environment)"
else
    echo "❌ ERROR: PARKS_APP_URL is required!"
    echo ""
    echo "Usage: $0 <app-url>"
    echo ""
    echo "Examples:"
    echo "  $0 http://restify-parks-app.apps.wdh418arm.migration.redhat.com"
    echo "  $0 http://localhost:8080"
    echo ""
    echo "Or set environment variable first:"
    echo "  export PARKS_APP_URL=http://your-app-url"
    echo "  $0"
    exit 1
fi

echo ""
echo "🧪 Running tests..."
echo ""

# Run the tests
python3 test_clicks.py

# Capture exit code
EXIT_CODE=$?

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ All tests passed!"
else
    echo "❌ Some tests failed (exit code: $EXIT_CODE)"
fi

exit $EXIT_CODE

