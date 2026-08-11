Feature: Time window navigation
  As an operator
  I want to zoom and pan the chart with the keyboard
  So that I can move between a spike and its context without a mouse

  Background:
    Given a buffer holding 15 minutes at 15s resolution
    And the window is 5 minutes wide and attached to live

  Scenario: Zooming in animates rather than snapping
    When I press "j"
    Then the target window is 1m
    And the drawn window is between 1m and 5m
    And it settles at 1m

  Scenario: Zooming out stops at the buffer horizon
    When I press "k" 6 times
    Then the target window is 15m
    And the buffer horizon is reported

  Scenario: Panning back detaches from live
    When I press "h" 3 times
    Then the window is detached from live
    And the window sits behind live

  Scenario: Panning forward reattaches to live
    Given the window is detached from live
    When I press "l" 20 times
    Then the window is attached to live

  Scenario: Jumping to the oldest bucket stays inside the buffer
    When I press "g"
    Then the window covers 20 buckets
    And the window stays inside the buffer

  Scenario: A missing bucket is not interpolated
    Given a series with a gap in the middle
    When the chart is drawn
    Then the line breaks across the gap

  @wip
  Scenario: Panning into spilled history
    Given buckets older than the memory window have spilled to temporary files
    When I pan into them
    Then they are read back and rendered
