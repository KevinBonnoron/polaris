import CodeMirror from '@uiw/react-codemirror';
import { githubDark } from '@uiw/codemirror-theme-github';
import { cpp } from '@codemirror/lang-cpp';
import { css } from '@codemirror/lang-css';
import { html } from '@codemirror/lang-html';
import { java } from '@codemirror/lang-java';
import { javascript } from '@codemirror/lang-javascript';
import { json } from '@codemirror/lang-json';
import { markdown } from '@codemirror/lang-markdown';
import { php } from '@codemirror/lang-php';
import { python } from '@codemirror/lang-python';
import { rust } from '@codemirror/lang-rust';
import { sql } from '@codemirror/lang-sql';
import { xml } from '@codemirror/lang-xml';
import { yaml } from '@codemirror/lang-yaml';
import { StreamLanguage } from '@codemirror/language';
import { go } from '@codemirror/legacy-modes/mode/go';
import { ruby } from '@codemirror/legacy-modes/mode/ruby';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import type { Extension } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { ChevronDown, ChevronRight, File, Folder } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { ListProjectDir, ReadProjectFile, WriteProjectFile } from '@/wailsjs/go/main/App';
import type { main } from '@/wailsjs/go/models';

type Entry = main.CodeEntry;

function getLanguageExtension(path: string): Extension {
  const ext = path.split('.').pop()?.toLowerCase() ?? '';
  switch (ext) {
    case 'ts':
      return javascript({ typescript: true });
    case 'tsx':
      return javascript({ typescript: true, jsx: true });
    case 'jsx':
      return javascript({ jsx: true });
    case 'js':
    case 'mjs':
    case 'cjs':
      return javascript();
    case 'go':
      return StreamLanguage.define(go);
    case 'py':
      return python();
    case 'rs':
      return rust();
    case 'css':
    case 'scss':
    case 'less':
      return css();
    case 'html':
    case 'htm':
      return html();
    case 'xml':
    case 'svg':
      return xml();
    case 'json':
      return json();
    case 'yaml':
    case 'yml':
      return yaml();
    case 'md':
      return markdown();
    case 'sql':
      return sql();
    case 'java':
      return java();
    case 'c':
    case 'h':
    case 'cpp':
    case 'cc':
    case 'hpp':
      return cpp();
    case 'php':
      return php();
    case 'rb':
      return StreamLanguage.define(ruby);
    case 'sh':
    case 'bash':
    case 'zsh':
      return StreamLanguage.define(shell);
    default:
      return [];
  }
}

const sizeTheme = EditorView.theme({
  '&': { fontSize: '12px', height: 'auto' },
  '.cm-scroller': { overflow: 'visible', fontFamily: 'var(--font-mono, monospace)' },
});

interface TreeNode {
  entry: Entry;
  children: TreeNode[] | null;
  expanded: boolean;
  loaded: boolean;
}

function buildRoot(entries: Entry[]): TreeNode[] {
  return (entries ?? [])
    .slice()
    .sort((a, b) => {
      if (a.isDir !== b.isDir) {
        return a.isDir ? -1 : 1;
      }
      return a.name.localeCompare(b.name);
    })
    .map((e) => ({ entry: e, children: null, expanded: false, loaded: false }));
}

function filterNodes(nodes: TreeNode[], query: string): TreeNode[] {
  if (!query) {
    return nodes;
  }
  const q = query.toLowerCase();
  return nodes.flatMap((n) => {
    if (n.entry.isDir) {
      const filtered = filterNodes(n.children ?? [], q);
      if (filtered.length === 0) {
        return [];
      }
      return [{ ...n, expanded: true, children: filtered }];
    }
    return n.entry.name.toLowerCase().includes(q) ? [n] : [];
  });
}

interface FilesTabProps {
  projectPath: string;
}

export function FilesTab({ projectPath }: FilesTabProps) {
  const { t } = useTranslation();
  const [roots, setRoots] = useState<TreeNode[]>([]);
  const [filter, setFilter] = useState('');
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [loadedContent, setLoadedContent] = useState<string | null>(null);
  const [contentError, setContentError] = useState<string | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [savedFlash, setSavedFlash] = useState(false);

  const editContentRef = useRef('');
  const savedContentRef = useRef('');

  useEffect(() => {
    ListProjectDir(projectPath, '')
      .then((entries) => setRoots(buildRoot(entries)))
      .catch((err) => toastError({ title: t('files.couldNotList'), err }));
  }, [projectPath, t]);

  const expand = useCallback(
    async (path: string) => {
      const load = async (nodes: TreeNode[]): Promise<TreeNode[]> =>
        Promise.all(
          nodes.map(async (n) => {
            if (n.entry.path === path) {
              if (n.loaded) {
                return { ...n, expanded: !n.expanded };
              }
              try {
                const entries = await ListProjectDir(projectPath, path);
                return { ...n, expanded: true, loaded: true, children: buildRoot(entries) };
              } catch (err) {
                toastError({ title: t('files.couldNotList'), err });
                return n;
              }
            }
            if (n.children) {
              return { ...n, children: await load(n.children) };
            }
            return n;
          }),
        );
      setRoots(await load(roots));
    },
    [roots, projectPath, t],
  );

  const selectFile = useCallback(
    async (path: string) => {
      setSelectedPath(path);
      setLoadedContent(null);
      setContentError(null);
      setLoadingContent(true);
      setDirty(false);
      setSavedFlash(false);
      editContentRef.current = '';
      savedContentRef.current = '';
      try {
        const text = await ReadProjectFile(projectPath, path);
        editContentRef.current = text;
        savedContentRef.current = text;
        setLoadedContent(text);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        if (msg.includes('too large')) {
          setContentError(t('files.fileTooLarge'));
        } else {
          setContentError(t('files.couldNotRead'));
          toastError({ title: t('files.couldNotRead'), err });
        }
      } finally {
        setLoadingContent(false);
      }
    },
    [projectPath, t],
  );

  const save = useCallback(async () => {
    if (!selectedPath || saving) {
      return;
    }
    setSaving(true);
    try {
      await WriteProjectFile(projectPath, selectedPath, editContentRef.current);
      savedContentRef.current = editContentRef.current;
      setDirty(false);
      setSavedFlash(true);
      setTimeout(() => setSavedFlash(false), 1500);
    } catch (err) {
      toastError({ title: t('files.couldNotSave'), err });
    } finally {
      setSaving(false);
    }
  }, [selectedPath, saving, projectPath, t]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        save();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [save]);

  const extensions = useMemo(() => [getLanguageExtension(selectedPath ?? ''), sizeTheme, EditorView.lineWrapping], [selectedPath]);

  const visible = useMemo(() => filterNodes(roots, filter), [roots, filter]);

  return (
    <div className="flex h-full min-h-0 flex-1 gap-0">
      <div className="flex h-full w-64 min-w-0 shrink-0 flex-col border-r border-border">
        <div className="shrink-0 p-2">
          <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('files.filterPlaceholder')} className="h-7 text-xs" />
        </div>
        <ScrollArea className="h-0 flex-1">
          <ul className="py-1">
            <NodeList nodes={visible} depth={0} selectedPath={selectedPath} onExpand={expand} onSelect={selectFile} />
          </ul>
        </ScrollArea>
      </div>

      <div className="flex h-full min-w-0 flex-1 flex-col">
        {selectedPath && loadedContent !== null && (
          <div className="flex shrink-0 items-center gap-1.5 border-b border-border px-3 py-1 text-[11px] text-muted-foreground">
            {dirty && <span className="text-amber-400">●</span>}
            {savedFlash && !dirty && <span className="text-green-400">✓</span>}
            <span className="truncate font-mono">{selectedPath.split('/').pop()}</span>
          </div>
        )}

        {!selectedPath && <p className="p-6 text-sm text-muted-foreground">{t('files.selectFile')}</p>}
        {loadingContent && <p className="p-6 text-sm text-muted-foreground">Loading…</p>}
        {contentError && <p className="p-6 text-sm text-destructive">{contentError}</p>}

        {loadedContent !== null && !loadingContent && selectedPath && (
          <ScrollArea className="min-h-0 flex-1" viewportProps={{ className: 'p-2' }}>
            <CodeMirror
              key={selectedPath}
              value={loadedContent}
              theme={githubDark}
              extensions={extensions}
              basicSetup={{ lineNumbers: true, foldGutter: false }}
              onChange={(val) => {
                editContentRef.current = val;
                setDirty(val !== savedContentRef.current);
              }}
            />
          </ScrollArea>
        )}
      </div>
    </div>
  );
}

interface NodeListProps {
  nodes: TreeNode[];
  depth: number;
  selectedPath: string | null;
  onExpand: (path: string) => void;
  onSelect: (path: string) => void;
}

function NodeList({ nodes, depth, selectedPath, onExpand, onSelect }: NodeListProps) {
  return (
    <>
      {nodes.map((n) => (
        <NodeRow key={n.entry.path} node={n} depth={depth} selectedPath={selectedPath} onExpand={onExpand} onSelect={onSelect} />
      ))}
    </>
  );
}

interface NodeRowProps {
  node: TreeNode;
  depth: number;
  selectedPath: string | null;
  onExpand: (path: string) => void;
  onSelect: (path: string) => void;
}

function NodeRow({ node, depth, selectedPath, onExpand, onSelect }: NodeRowProps) {
  const isSelected = !node.entry.isDir && selectedPath === node.entry.path;

  return (
    <>
      <li>
        <button
          type="button"
          style={{ paddingLeft: `${8 + depth * 12}px` }}
          onClick={() => (node.entry.isDir ? onExpand(node.entry.path) : onSelect(node.entry.path))}
          className={cn('flex w-full items-center gap-1.5 py-0.5 pr-2 text-left text-xs hover:bg-accent hover:text-accent-foreground', isSelected && 'bg-accent text-accent-foreground')}
        >
          {node.entry.isDir ? (
            <>
              {node.expanded ? <ChevronDown className="size-3 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-3 shrink-0 text-muted-foreground" />}
              <Folder className="size-3.5 shrink-0 text-muted-foreground" />
            </>
          ) : (
            <>
              <span className="size-3 shrink-0" />
              <File className="size-3.5 shrink-0 text-muted-foreground" />
            </>
          )}
          <span className="truncate">{node.entry.name}</span>
        </button>
      </li>
      {node.entry.isDir && node.expanded && node.children && <NodeList nodes={node.children} depth={depth + 1} selectedPath={selectedPath} onExpand={onExpand} onSelect={onSelect} />}
    </>
  );
}
