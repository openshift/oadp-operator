#!/usr/bin/env python3
"""
Test suite for the Parks App Click Tracking functionality

This test suite validates:
- GET /clicks - Retrieve click count
- POST /clicks - Record a new click
- Click count increments correctly
- Response format validation
"""

import unittest
import requests
import json
import os
from time import sleep


class TestClickTracking(unittest.TestCase):
    """Test suite for click tracking endpoints"""

    @classmethod
    def setUpClass(cls):
        """Set up test configuration once for all tests"""
        # Get base URL from environment variable (required)
        cls.base_url = os.environ.get('PARKS_APP_URL')
        
        if not cls.base_url:
            raise ValueError(
                "PARKS_APP_URL environment variable is required.\n"
                "Set it via: export PARKS_APP_URL=http://your-app-url\n"
                "Or run with: python test_clicks.py --url http://your-app-url"
            )
        
        cls.clicks_endpoint = f"{cls.base_url}/clicks"
        print(f"\n{'='*70}")
        print(f"Testing Click Tracking at: {cls.clicks_endpoint}")
        print(f"{'='*70}\n")

    def test_01_get_clicks_endpoint_exists(self):
        """Test that the GET /clicks endpoint exists and returns 200"""
        response = requests.get(self.clicks_endpoint)
        self.assertEqual(
            response.status_code,
            200,
            f"Expected 200 OK, got {response.status_code}"
        )
        print(f"✓ GET /clicks endpoint exists and returns 200")

    def test_02_get_clicks_returns_json(self):
        """Test that GET /clicks returns valid JSON"""
        response = requests.get(self.clicks_endpoint)
        self.assertEqual(response.headers.get('Content-Type'), 'application/json')
        
        # Verify we can parse the JSON
        try:
            data = response.json()
            print(f"✓ GET /clicks returns valid JSON: {data}")
        except json.JSONDecodeError:
            self.fail("Response is not valid JSON")

    def test_03_get_clicks_has_total_clicks_field(self):
        """Test that GET /clicks response contains 'totalClicks' field"""
        response = requests.get(self.clicks_endpoint)
        data = response.json()
        
        self.assertIn(
            'totalClicks',
            data,
            "Response should contain 'totalClicks' field"
        )
        self.assertIsInstance(
            data['totalClicks'],
            int,
            "totalClicks should be an integer"
        )
        print(f"✓ Response contains 'totalClicks' field with value: {data['totalClicks']}")

    def test_04_post_clicks_increments_count(self):
        """Test that POST /clicks increments the click count"""
        # Get initial count
        initial_response = requests.get(self.clicks_endpoint)
        initial_count = initial_response.json()['totalClicks']
        print(f"  Initial count: {initial_count}")
        
        # Post a new click
        post_response = requests.post(self.clicks_endpoint)
        self.assertEqual(
            post_response.status_code,
            200,
            f"Expected 200 OK for POST, got {post_response.status_code}"
        )
        
        # Verify post response contains the new count
        post_data = post_response.json()
        self.assertIn('totalClicks', post_data, "POST response should contain totalClicks")
        post_count = post_data['totalClicks']
        print(f"  Count after POST: {post_count}")
        
        # Verify the count incremented by exactly 1
        self.assertEqual(
            post_count,
            initial_count + 1,
            f"Click count should increment by 1. Expected {initial_count + 1}, got {post_count}"
        )
        
        # Verify GET returns the same count
        sleep(0.1)  # Small delay to ensure DB write completed
        final_response = requests.get(self.clicks_endpoint)
        final_count = final_response.json()['totalClicks']
        
        self.assertEqual(
            final_count,
            post_count,
            f"GET should return same count as POST. Expected {post_count}, got {final_count}"
        )
        print(f"✓ POST /clicks successfully incremented count from {initial_count} to {final_count}")

    def test_05_post_clicks_returns_success(self):
        """Test that POST /clicks returns success flag"""
        response = requests.post(self.clicks_endpoint)
        data = response.json()
        
        # Check for success field (may or may not be present depending on implementation)
        if 'success' in data:
            self.assertTrue(
                data['success'],
                "success field should be true"
            )
            print(f"✓ POST response contains success=true")
        else:
            print(f"✓ POST response format: {data}")

    def test_06_multiple_posts_increment_correctly(self):
        """Test that multiple POST requests increment count correctly"""
        # Get initial count
        initial_response = requests.get(self.clicks_endpoint)
        initial_count = initial_response.json()['totalClicks']
        print(f"  Initial count: {initial_count}")
        
        # Post multiple clicks
        num_clicks = 3
        for i in range(num_clicks):
            response = requests.post(self.clicks_endpoint)
            self.assertEqual(response.status_code, 200)
            sleep(0.05)  # Small delay between requests
        
        # Get final count
        sleep(0.2)  # Ensure all writes completed
        final_response = requests.get(self.clicks_endpoint)
        final_count = final_response.json()['totalClicks']
        
        # Verify count increased by num_clicks
        self.assertEqual(
            final_count,
            initial_count + num_clicks,
            f"After {num_clicks} POSTs, count should be {initial_count + num_clicks}, got {final_count}"
        )
        print(f"✓ {num_clicks} POST requests correctly incremented count from {initial_count} to {final_count}")

    def test_07_concurrent_clicks_handled(self):
        """Test that concurrent clicks are handled properly"""
        import concurrent.futures
        
        # Get initial count
        initial_response = requests.get(self.clicks_endpoint)
        initial_count = initial_response.json()['totalClicks']
        print(f"  Initial count: {initial_count}")
        
        # Function to post a click
        def post_click():
            return requests.post(self.clicks_endpoint)
        
        # Post 5 clicks concurrently
        num_concurrent_clicks = 5
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(post_click) for _ in range(num_concurrent_clicks)]
            results = [f.result() for f in concurrent.futures.as_completed(futures)]
        
        # Verify all requests succeeded
        for result in results:
            self.assertEqual(result.status_code, 200)
        
        # Get final count
        sleep(0.5)  # Ensure all writes completed
        final_response = requests.get(self.clicks_endpoint)
        final_count = final_response.json()['totalClicks']
        
        # Verify count increased by num_concurrent_clicks
        self.assertEqual(
            final_count,
            initial_count + num_concurrent_clicks,
            f"After {num_concurrent_clicks} concurrent POSTs, count should be {initial_count + num_concurrent_clicks}, got {final_count}"
        )
        print(f"✓ {num_concurrent_clicks} concurrent clicks correctly handled: {initial_count} → {final_count}")

    def test_08_get_clicks_is_idempotent(self):
        """Test that GET /clicks doesn't change the count"""
        # Get initial count
        first_response = requests.get(self.clicks_endpoint)
        first_count = first_response.json()['totalClicks']
        
        # Make multiple GET requests
        for _ in range(5):
            response = requests.get(self.clicks_endpoint)
            current_count = response.json()['totalClicks']
            self.assertEqual(
                current_count,
                first_count,
                "GET requests should not change the click count"
            )
        
        print(f"✓ GET /clicks is idempotent (count remained {first_count})")


class TestParksEndpoint(unittest.TestCase):
    """Test suite for the parks data endpoint"""

    @classmethod
    def setUpClass(cls):
        """Set up test configuration"""
        # Get base URL from environment variable (required)
        cls.base_url = os.environ.get('PARKS_APP_URL')
        
        if not cls.base_url:
            raise ValueError(
                "PARKS_APP_URL environment variable is required.\n"
                "Set it via: export PARKS_APP_URL=http://your-app-url\n"
                "Or run with: python test_clicks.py --url http://your-app-url"
            )
        
        cls.parks_endpoint = f"{cls.base_url}/parks"
        print(f"\n{'='*70}")
        print(f"Testing Parks Data at: {cls.parks_endpoint}")
        print(f"{'='*70}\n")

    def test_10_parks_endpoint_returns_data(self):
        """Test that GET /parks returns park data"""
        response = requests.get(self.parks_endpoint)
        self.assertEqual(
            response.status_code,
            200,
            f"Expected 200 OK, got {response.status_code}"
        )
        
        # Verify we get JSON data
        data = response.json()
        self.assertIsInstance(data, list, "Parks data should be a list")
        self.assertGreater(len(data), 0, "Parks data should not be empty")
        print(f"✓ GET /parks returns {len(data)} parks")

    def test_11_parks_contains_creek_battlefield(self):
        """Test that 'Creek National Battlefield' is in the parks data"""
        response = requests.get(self.parks_endpoint)
        self.assertEqual(response.status_code, 200, "Parks endpoint should return 200")
        
        data = response.json()
        self.assertIsInstance(data, list, "Parks data should be a list")
        
        # Search for Creek National Battlefield
        creek_battlefield_found = False
        matching_park = None
        
        for park in data:
            # Check if park has a Name field
            if 'Name' in park:
                if 'Creek National Battlefield' in park['Name']:
                    creek_battlefield_found = True
                    matching_park = park
                    break
        
        self.assertTrue(
            creek_battlefield_found,
            "Parks data should contain 'Creek National Battlefield'"
        )
        
        print(f"✓ Found park: {matching_park['Name']}")
        
        # Additional validation - verify park has required fields
        if matching_park:
            self.assertIn('Name', matching_park, "Park should have 'Name' field")
            self.assertIn('pos', matching_park, "Park should have 'pos' (position) field")
            print(f"  Position: {matching_park['pos']}")


class TestClickTrackingErrorHandling(unittest.TestCase):
    """Test suite for error handling in click tracking"""

    @classmethod
    def setUpClass(cls):
        """Set up test configuration"""
        # Get base URL from environment variable (required)
        cls.base_url = os.environ.get('PARKS_APP_URL')
        
        if not cls.base_url:
            raise ValueError(
                "PARKS_APP_URL environment variable is required.\n"
                "Set it via: export PARKS_APP_URL=http://your-app-url\n"
                "Or run with: python test_clicks.py --url http://your-app-url"
            )
        
        cls.clicks_endpoint = f"{cls.base_url}/clicks"

    def test_12_invalid_methods_rejected(self):
        """Test that invalid HTTP methods are rejected appropriately"""
        # Test PUT (should not be allowed)
        put_response = requests.put(self.clicks_endpoint)
        self.assertIn(
            put_response.status_code,
            [405, 404],  # Method Not Allowed or Not Found
            f"PUT should be rejected, got {put_response.status_code}"
        )
        print(f"✓ PUT method rejected with status {put_response.status_code}")
        
        # Test DELETE (should not be allowed)
        delete_response = requests.delete(self.clicks_endpoint)
        self.assertIn(
            delete_response.status_code,
            [405, 404],
            f"DELETE should be rejected, got {delete_response.status_code}"
        )
        print(f"✓ DELETE method rejected with status {delete_response.status_code}")


def run_tests():
    """Run the test suite with verbose output"""
    # Create test suite
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()
    
    # Add test classes
    suite.addTests(loader.loadTestsFromTestCase(TestClickTracking))
    suite.addTests(loader.loadTestsFromTestCase(TestParksEndpoint))
    suite.addTests(loader.loadTestsFromTestCase(TestClickTrackingErrorHandling))
    
    # Run tests with verbose output
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    
    # Print summary
    print(f"\n{'='*70}")
    print(f"Test Summary:")
    print(f"  Tests Run: {result.testsRun}")
    print(f"  Successes: {result.testsRun - len(result.failures) - len(result.errors)}")
    print(f"  Failures: {len(result.failures)}")
    print(f"  Errors: {len(result.errors)}")
    print(f"{'='*70}\n")
    
    return result


if __name__ == '__main__':
    # Allow running as script or with unittest
    import sys
    import argparse
    
    # Parse custom arguments before unittest gets them
    parser = argparse.ArgumentParser(
        description='Run Parks App test suite',
        epilog='Example: python test_clicks.py --url http://localhost:8080'
    )
    parser.add_argument(
        '--url',
        help='Base URL of the Parks App (e.g., http://localhost:8080)',
        required=False
    )
    
    # Parse known args, leave the rest for unittest
    args, remaining_argv = parser.parse_known_args()
    
    # Set environment variable if --url was provided
    if args.url:
        os.environ['PARKS_APP_URL'] = args.url
        print(f"Using URL from command-line: {args.url}\n")
    elif not os.environ.get('PARKS_APP_URL'):
        print("ERROR: PARKS_APP_URL must be set!")
        print("\nOptions:")
        print("  1. Via environment variable:")
        print("     export PARKS_APP_URL=http://your-app-url")
        print("     python test_clicks.py")
        print("\n  2. Via command-line argument:")
        print("     python test_clicks.py --url http://your-app-url")
        print("\nExamples:")
        print("  python test_clicks.py --url http://restify-parks-app.apps.wdh418arm.migration.redhat.com")
        print("  python test_clicks.py --url http://localhost:8080")
        sys.exit(1)
    
    # Update sys.argv for unittest
    sys.argv = [sys.argv[0]] + remaining_argv
    
    run_tests()

