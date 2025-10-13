# Parks App Test Suite

This directory contains automated tests for the Parks App, specifically testing the click tracking functionality.

## Test Files

- `test_clicks.py` - Comprehensive test suite for click tracking endpoints

## Prerequisites

```bash
# Install Python dependencies
pip install -r requirements.txt
```

## Running the Tests

**IMPORTANT**: The `PARKS_APP_URL` environment variable or `--url` argument is **required**.

### Method 1: Using Environment Variable

```bash
# Set the environment variable
export PARKS_APP_URL=http://restify-parks-app.apps.wdh418arm.migration.redhat.com

# Run the tests
python test_clicks.py

# Or using unittest
python -m unittest test_clicks.py -v
```

### Method 2: Using Command-Line Argument

```bash
# Test against production
python test_clicks.py --url http://restify-parks-app.apps.wdh418arm.migration.redhat.com

# Test against local deployment
python test_clicks.py --url http://localhost:8080

# Test against custom deployment
python test_clicks.py --url http://your-custom-url.com
```

### Method 3: Using the Test Runner Script

```bash
# The script requires a URL argument
./run_tests.sh http://restify-parks-app.apps.wdh418arm.migration.redhat.com

# Or against localhost
./run_tests.sh http://localhost:8080
```

### Using pytest (optional)

```bash
# Install pytest if you prefer
pip install pytest

# Run with pytest
pytest test_clicks.py -v
```

## Test Coverage

The test suite validates:

### Click Tracking Tests (`TestClickTracking`)
1. ✓ GET /clicks endpoint exists and returns 200
2. ✓ GET /clicks returns valid JSON
3. ✓ Response contains 'totalClicks' field
4. ✓ POST /clicks increments the click count
5. ✓ POST response includes success flag
6. ✓ Multiple POST requests increment correctly
7. ✓ Concurrent clicks are handled properly
8. ✓ GET requests are idempotent (don't change count)

### Parks Data Tests (`TestParksEndpoint`)
9. ✓ GET /parks endpoint returns park data
10. ✓ Parks data contains "Creek National Battlefield"

### Error Handling Tests (`TestClickTrackingErrorHandling`)
11. ✓ Invalid HTTP methods (PUT, DELETE) are rejected

## Example Output

```
======================================================================
Testing Click Tracking at: http://restify-parks-app.apps.wdh418arm.migration.redhat.com/clicks
======================================================================

test_01_get_clicks_endpoint_exists (test_clicks.TestClickTracking) ... ✓ GET /clicks endpoint exists and returns 200
ok
test_02_get_clicks_returns_json (test_clicks.TestClickTracking) ... ✓ GET /clicks returns valid JSON: {'totalClicks': 6}
ok
test_03_get_clicks_has_total_clicks_field (test_clicks.TestClickTracking) ... ✓ Response contains 'totalClicks' field with value: 6
ok
test_04_post_clicks_increments_count (test_clicks.TestClickTracking) ...   Initial count: 6
  Count after POST: 7
✓ POST /clicks successfully incremented count from 6 to 7
ok

...

======================================================================
Test Summary:
  Tests Run: 11
  Successes: 11
  Failures: 0
  Errors: 0
======================================================================
```

## Quick Test Commands

```bash
# Set URL first (required for all commands below)
export PARKS_APP_URL=http://restify-parks-app.apps.wdh418arm.migration.redhat.com

# Quick smoke test - single POST and verify
curl -X POST $PARKS_APP_URL/clicks
curl $PARKS_APP_URL/clicks

# Run just the basic tests
python -m unittest test_clicks.TestClickTracking.test_01_get_clicks_endpoint_exists -v
python -m unittest test_clicks.TestClickTracking.test_04_post_clicks_increments_count -v

# Run just the parks data test
python -m unittest test_clicks.TestParksEndpoint.test_11_parks_contains_creek_battlefield -v

# Run all parks tests
python -m unittest test_clicks.TestParksEndpoint -v

# Or use --url with each command
python test_clicks.py --url $PARKS_APP_URL
```

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Install test dependencies
  run: pip install -r tests/requirements.txt

- name: Run click tracking tests
  run: python tests/test_clicks.py
  env:
    PARKS_APP_URL: ${{ secrets.APP_URL }}
```

### Jenkins Example

```groovy
stage('Test') {
    steps {
        sh 'pip install -r tests/requirements.txt'
        sh 'python tests/test_clicks.py'
    }
}
```

## Troubleshooting

### Connection Errors
If you get connection errors, verify:
1. The app is deployed and accessible
2. The URL is correct
3. Network connectivity is available

### Test Failures
If tests fail:
1. Check that MongoDB is running and connected
2. Verify the `/clicks` endpoints are properly configured
3. Check server logs for errors
4. Ensure the database can write data

### Debugging
Enable verbose output:
```bash
python test_clicks.py -v
```

Or add print statements in test methods for more details.

