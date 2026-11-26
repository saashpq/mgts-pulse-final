import json
import unittest
from pathlib import Path


class SavedReviewTest(unittest.TestCase):
    def test_published_run_covers_every_role_and_case_once(self):
        folder = Path(__file__).resolve().parent
        spec = json.loads((folder / 'scenarios.json').read_text('utf-8'))
        saved = json.loads((folder / 'review-2026-11-26.json').read_text('utf-8'))
        required = {(case['id'], role) for case in spec['cases'] for role in spec['roles']}
        self.assertEqual(len(required), 48)
        rows = saved['reviews']
        self.assertEqual(len(rows), len(required))
        self.assertEqual({(row['case'], row['role']) for row in rows}, required)
        for row in rows:
            self.assertEqual(len(row['read']), 2)
            self.assertEqual(len(set(row['read'])), 2)
            self.assertTrue(set(row['read']) <= {'record', 'counterexample', 'metric'})
            self.assertIn(row['basis'], row['read'])
            self.assertTrue(row['action'].strip() and row['check'].strip())
