import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const root = new URL('./', import.meta.url);
const html = await readFile(new URL('index.html', root), 'utf8');
const script = await readFile(new URL('timekeeper.js', root), 'utf8');
const css = await readFile(new URL('timekeeper.css', root), 'utf8');

test('Trail of Thought board exposes project-scoped compose controls', () => {
  for (const marker of ['messages-project', 'message-compose-form', 'message-compose-body', 'message-compose-kind', 'message-compose-tags', 'message-compose-link', 'message-compose-count']) {
    assert.match(html, new RegExp('id="' + marker + '"'));
  }
  assert.match(html, /maxlength="1000"/);
  assert.match(html, /Trail of Thought Board/);
});

test('Trail of Thought board supports reply, edit, revisions, and project switching', () => {
  assert.match(script, /beginReply/);
  assert.match(script, /beginEdit/);
  assert.match(script, /messagePath\(messagesProjectID, id, 'revisions'\)/);
  assert.match(script, /messagePath\(messagesProjectID, id, 'context'\)/);
  assert.match(script, /messagePath\(messagesProjectID, id, 'pin'\)/);
  assert.match(script, /related_mode/);
  assert.match(script, /pinned/);
  assert.match(script, /Context/);
  assert.match(script, /parent_message_id/);
  assert.match(script, /method: 'PATCH'/);
  assert.match(script, /messagesProjectSelect/);
  assert.match(script, /messagesLoadToken/);
});

test('Trail of Thought board styles nested messages and compact composition', () => {
  assert.match(css, /\.message-compose/);
  assert.match(css, /\.message-actions/);
  assert.match(css, /\.message-revisions/);
  assert.match(css, /\.message-context/);
  assert.match(css, /\.message-pin/);
  assert.match(css, /\.messages-project/);
});
