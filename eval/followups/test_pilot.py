import unittest
from pilot import summarize, render


class PilotReportTest(unittest.TestCase):
    def test_missed_then_rescheduled_still_counts(self):
        moves = [
            {'due_date':'2026-11-23','next_due_date':'2026-11-25','at':'2026-11-23T21:01:00Z'},
            {'due_date':'2026-11-25','next_due_date':'2026-11-29','at':'2026-11-25T12:00:00Z'}
        ]
        result = summarize({'today':'2026-11-26','items':[
            {'status':'open','due_date':'2026-11-29','checks':[],'reschedules':moves}
        ]})
        self.assertEqual((result['due_contacts'], result['on_time_records'], result['moved_after_deadline']), (1,0,1))

    def test_empty_and_explicit_denominators(self):
        self.assertIn('нет наблюдений',render(summarize({'today':'2026-11-26','items':[]})))
        history=[
            {'due_date':'2026-11-24','at':'2026-11-24T21:30:00Z','outcome':'unknown','delivery':'done'},
            {'due_date':'2026-11-25','at':'2026-11-25T20:58:00Z','outcome':'partial','delivery':'done'}
        ]
        result=summarize({'today':'2026-11-26','items':[
            {'status':'closed','due_date':'2026-11-25','checks':history},
            {'status':'open','due_date':'2026-11-21','checks':[]},
            {'status':'open','due_date':'2026-11-26','checks':[]},
            {'status':'open','due_date':'2026-12-01','checks':[]}
        ]})
        self.assertEqual((result['due_contacts'],result['open_overdue']), (4,1))
        self.assertEqual((result['on_time_records'],result['on_time_known_outcomes']),(1,1))
        self.assertEqual((result['action_completed_at_contact'],result['known_outcomes']),(2,1))
        self.assertEqual(result['reported_outcomes']['done'],0)
