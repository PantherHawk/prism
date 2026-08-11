Feature: Theme
  As an operator
  I want prism to match the terminal I am already looking at
  So that a dashboard on a light background is readable rather than a guess

  Scenario: Auto follows a dark terminal
    Given the theme mode is "auto"
    When the terminal reports a dark background
    Then the dark palette is in force

  Scenario: Auto follows a light terminal
    Given the theme mode is "auto"
    When the terminal reports a light background
    Then the light palette is in force

  Scenario: A configured dark theme overrides the terminal
    Given the theme mode is "dark"
    When the terminal reports a light background
    Then the dark palette is in force

  Scenario: A configured light theme overrides the terminal
    Given the theme mode is "light"
    When the terminal reports a dark background
    Then the light palette is in force

  # The theme key is d, bound in the TUI keymap.
  Scenario: The operator toggles the theme
    Given the theme mode is "auto"
    And the terminal reports a dark background
    When I press the theme key
    Then the light palette is in force

  # Pressing d is a decision. A background report arriving afterwards must not
  # snap the palette back, or the toggle reads as having failed.
  Scenario: A toggle survives a later terminal report
    Given the theme mode is "auto"
    And the terminal reports a dark background
    And I press the theme key
    When the terminal reports a dark background
    Then the light palette is in force

  Scenario: An unreadable theme is rejected
    Given the theme mode is "sepia"
    Then the theme is rejected naming "sepia"

  Scenario: Both palettes fill every role
    Given the theme mode is "dark"
    Then every colour role is set
    And the spectrum has the same number of bands in both palettes

  # The acceptance criterion for this phase, executed rather than described:
  # every colour goes through theme.Palette, so a hex literal anywhere else is
  # a colour decision that light mode will not reach.
  Scenario: Colours live only in the theme package
    Then no Go file outside internal/theme contains a hex colour
