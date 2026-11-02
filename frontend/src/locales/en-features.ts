// Feature panel copy: todo / goal / plan / memory live state plus the config drawer.
// Split from en-pages.ts for the same reason as the Chinese file: these four grow with the features.
export default {
  featurePanel: {
    tabTodo: 'Todo',
    tabGoal: 'Goal',
    tabPlan: 'Plan',

    todoEmpty: 'No todos right now',
    todoEmptyHint: 'When the assistant breaks a multi-step task down, the list shows up here',
    todoProgress: '{done} of {total} done',
    todoBlocked: 'Blocked',
    todoBlockedHint: 'An item it depends on is still open, so it cannot start yet',

    goalEmpty: 'No goal right now',
    goalEmptyHint:
      'With goal tracking on, the assistant can register a long multi-turn task as a goal and resume it',
    goalProgress: '{done} of {total} steps',
    goalResume: 'auto-resumed {n} times',
    goalStatus: {
      active: 'Active',
      blocked: 'Blocked',
      achieved: 'Achieved',
      abandoned: 'Abandoned',
    },

    planEmpty: 'No plan right now',
    planEmptyHint:
      'With plan review on, the assistant submits a step list for approval before acting',
    planProgress: '{done} of {total} steps done',
    planRejected: 'Rejected: {note}',
    planNotes: 'Plan-level notes',
    planStepNote: 'Note: {text}',
    planStatus: {
      draft: 'Awaiting approval',
      approved: 'Approved',
      rejected: 'Rejected',
      done: 'Done',
    },

    memoryEmpty: 'The memory store is empty',
    memoryEmptyHint: 'With persistent memory on, the assistant saves facts worth keeping across sessions',
    memoryAll: 'All',
    memoryFilterEmpty: 'Nothing under this category yet',
    memoryDelete: 'Delete this entry',
    memoryDeleted: 'Deleted',
    memoryKind: {
      memory: 'Facts',
      user: 'Preferences',
      project: 'Project',
      failure: 'Lessons',
    },

    configOpen: 'Configure',
    configTitle: '{name} · Settings',
    configSub: 'Changes take effect right after saving; tools are rebuilt',
    configNone: 'This feature has nothing to configure',
    configSaved: 'Settings saved',
  },
}
