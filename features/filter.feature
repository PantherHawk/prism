Feature: Filtering and pivoting
  As an operator
  I want to narrow a family and split it across a label
  So that I can see which component of a total is moving

  Background:
    Given a family "rq" with series
      | cluster | status | value |
      | api     | 200    | 100   |
      | api     | 500    | 20    |
      | auth    | 200    | 60    |
      | web     | 200    | 5     |

  Scenario: A label matcher narrows the set
    When the filter is "cluster=api"
    Then 2 series match

  Scenario: A regex matcher is anchored
    When the filter is "status=~2"
    Then 0 series match

  Scenario: Terms are combined with and
    When the filter is "cluster=~a.* status=200"
    Then 2 series match

  Scenario: A filter matching nothing is reported rather than blank
    When the filter is "cluster=nope"
    Then 0 series match
    And the filter is still reported as "cluster=nope"

  Scenario: An invalid filter is rejected with a reason
    When the filter is "cluster=~[unclosed"
    Then the filter is rejected
    And the reason mentions "invalid regular expression"

  Scenario: Pivoting splits the family across a label
    When pivoting on "cluster"
    Then there are 3 lines
    And the line "api" totals 120 across 2 series

  Scenario: Pivoting collapses the tail
    When pivoting on "cluster" with a limit of 2
    Then there are 2 lines
    And the last line is "other" totalling 65 across 2 series

  Scenario: A filter applies before the pivot
    When the filter is "status=200"
    And pivoting on "cluster"
    Then the line "api" totals 100 across 1 series
