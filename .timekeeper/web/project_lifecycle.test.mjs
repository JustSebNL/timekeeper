import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

const source = fs.readFileSync(new URL('./timekeeper.js', import.meta.url), 'utf8');
const html = fs.readFileSync(new URL('./index.html', import.meta.url), 'utf8');
function functionSource(name, nextName) {
  const start = source.indexOf(`  function ${name}(`);
  const end = source.indexOf(`  function ${nextName}(`, start + 1);
  assert.notEqual(start, -1, `${name} exists in dashboard source`);
  assert.notEqual(end, -1, `${nextName} follows ${name}`);
  return source.slice(start, end);
}

class Element {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.listeners = new Map();
    this.attributes = new Map();
    this.style = {};
    this.value = '';
    this.textContent = '';
    this.className = '';
    this.disabled = false;
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  setAttribute(name, value) { this.attributes.set(name, value); }
  addEventListener(name, callback) { this.listeners.set(name, callback); }
  async dispatch(name) {
    const callback = this.listeners.get(name);
    assert.ok(callback, `expected ${name} listener on ${this.tagName}`);
    await callback({ preventDefault() {} });
  }
  find(predicate) {
    if (predicate(this)) return this;
    for (const child of this.children) {
      const result = child.find?.(predicate);
      if (result) return result;
    }
    return null;
  }
}

function loadFunction(name, nextName, requests, refreshes) {
  const document = { createElement: tag => new Element(tag) };
  const request = async (url, options = {}) => {
    requests.push({ url, options });
    return {};
  };
  const refresh = async () => { refreshes.count++; };
  const make = new Function('document', 'request', 'api', functionSource(name, nextName) + `; return ${name};`);
  return { render: make(document, request, '/api/v1/projects'), refresh };
}

test('Project creation captures optional planned start and target end dates', () => {
  assert.match(html, /name="planned_start_date"[^>]*type="date"/);
  assert.match(html, /name="target_end_date"[^>]*type="date"/);
  assert.match(source, /planned_start_date: fields\.get\('planned_start_date'\)/);
  assert.match(source, /target_end_date: fields\.get\('target_end_date'\)/);
});

test('Dashboard deadline watch calls out overdue and due-soon items', () => {
  const document = { createElement: tag => new Element(tag) };
  const make = new Function('document', functionSource('renderProjectDeadlineWatch', 'filterProjectCards') + '; return renderProjectDeadlineWatch;');
  const renderDeadlineWatch = make(document);
  const target = new Element('div');
  renderDeadlineWatch({ items: [
    { project_id: 'P-1', project_name: 'Platform', entity_type: 'project', entity_id: 'P-1', name: 'Platform', target_date: '2026-10-09', deadline_state: 'overdue', status: 'Open', days_to_deadline: -1 },
    { project_id: 'P-2', project_name: 'Launch', entity_type: 'milestone', entity_id: 'MS-2', name: 'Beta gate', target_date: '2026-10-12', deadline_state: 'due_soon', status: 'Planned', days_to_deadline: 2 }
  ] }, target, new Element('div'));
  const text = target.find(node => node.tagName === 'UL')?.children.map(item => item.children.map(child => child.textContent).join(' ')).join('\n') || '';
  assert.match(text, /Platform.*OVERDUE.*2026-10-09/i);
  assert.match(text, /Beta gate.*DUE SOON.*2026-10-12/i);
  assert.equal(target.className, 'deadline-watch');
});

test('Project schedule controls persist dates and phase through API contracts', async () => {
  const requests = [];
  const refreshes = { count: 0 };
  const setup = loadFunction('renderProjectSchedule', 'renderProjectMilestones', requests, refreshes);
  const root = setup.render({ project_id: 'P-123', lifecycle_phase: 'planning', schedule_timezone: 'UTC', deadline_warning_days: 7 }, setup.refresh);
  const schedule = root.find(node => node.className.includes('project-schedule-form'));
  schedule.find(node => node.attributes.get('aria-label') === 'Planned Project start date').value = '2026-10-12';
  schedule.find(node => node.attributes.get('aria-label') === 'Target Project end date').value = '2026-11-12';
  schedule.find(node => node.attributes.get('aria-label') === 'Project schedule timezone').value = 'Europe/Amsterdam';
  schedule.find(node => node.attributes.get('aria-label') === 'Deadline warning days').value = '10';
  await schedule.dispatch('submit');
  assert.equal(requests[0].url, '/api/v1/projects/P-123/schedule');
  assert.equal(requests[0].options.method, 'PATCH');
  assert.deepEqual(JSON.parse(requests[0].options.body), {
    planned_start_date: '2026-10-12', target_end_date: '2026-11-12',
    schedule_timezone: 'Europe/Amsterdam', deadline_warning_days: 10
  });

  const phase = root.find(node => node.className.includes('project-phase-form'));
  phase.find(node => node.attributes.get('aria-label') === 'Project lifecycle phase').value = 'execution';
  phase.find(node => node.attributes.get('aria-label') === 'Project phase transition reason').value = 'Ready';
  await phase.dispatch('submit');
  assert.equal(requests[1].url, '/api/v1/projects/P-123/phase');
  assert.deepEqual(JSON.parse(requests[1].options.body), { phase: 'execution', reason: 'Ready' });
  assert.equal(refreshes.count, 2);
});

test('Milestone controls create and update dated checkpoints', async () => {
  const requests = [];
  const refreshes = { count: 0 };
  const setup = loadFunction('renderProjectMilestones', 'renderTaskMetadata', requests, refreshes);
  const root = setup.render({ project_id: 'P-123' }, [], setup.refresh);

  const create = root.find(node => node.className.includes('milestone-create-form'));
  create.find(node => node.attributes.get('aria-label') === 'Milestone name').value = 'Beta acceptance';
  create.find(node => node.attributes.get('aria-label') === 'Milestone target date').value = '2026-11-01';
  create.find(node => node.attributes.get('aria-label') === 'Milestone description').value = 'All acceptance probes pass';
  create.find(node => node.attributes.get('aria-label') === 'Optional linked Task ID').value = 'T-456';
  await create.dispatch('submit');
  assert.equal(requests[0].url, '/api/v1/projects/P-123/milestones');
  assert.equal(requests[0].options.method, 'POST');
  assert.deepEqual(JSON.parse(requests[0].options.body), {
    name: 'Beta acceptance', target_date: '2026-11-01',
    description: 'All acceptance probes pass', linked_task_id: 'T-456'
  });

  const existingRoot = setup.render({ project_id: 'P-123' }, [{ milestone_id: 'MS-789', name: 'Beta gate', target_date: '2026-10-22', status: 'Planned', deadline_state: 'due_soon' }], setup.refresh);
  const existing = existingRoot.find(node => node.className === 'milestone-row');
  existing.find(node => node.tagName === 'INPUT' && node.type === 'date').value = '2026-10-23';
  const status = existing.find(node => node.tagName === 'SELECT');
  status.value = 'Completed';
  await existing.find(node => node.tagName === 'BUTTON' && node.textContent === 'Save milestone').dispatch('click');
  assert.equal(requests[1].url, '/api/v1/projects/P-123/milestones/MS-789');
  assert.equal(requests[1].options.method, 'PATCH');
  assert.deepEqual(JSON.parse(requests[1].options.body), { target_date: '2026-10-23' });
  assert.equal(requests[2].url, '/api/v1/projects/P-123/milestones/MS-789/status');
  assert.deepEqual(JSON.parse(requests[2].options.body), { status: 'Completed', reason: '' });
  assert.equal(refreshes.count, 2);
});
