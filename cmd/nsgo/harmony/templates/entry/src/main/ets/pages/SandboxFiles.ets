import { util } from '@kit.ArkTS';
import { fileIo as fs } from '@kit.CoreFileKit';
import type { BusinessError } from '@kit.BasicServicesKit';
import fileio from '@ohos.fileio';

interface Request {
  path?: string;
  target?: string;
  fd?: number;
  flags?: number;
  mode?: number;
  position?: number | null;
  size?: number;
  data?: string;
}

const errorCodes: Map<number, string> = new Map<number, string>([
  [13900001, 'EPERM'], [13900002, 'ENOENT'], [13900004, 'EINTR'],
  [13900005, 'EIO'], [13900008, 'EBADF'], [13900010, 'EAGAIN'],
  [13900011, 'ENOMEM'], [13900012, 'EACCES'], [13900014, 'EBUSY'],
  [13900015, 'EEXIST'], [13900016, 'EXDEV'], [13900018, 'ENOTDIR'],
  [13900019, 'EISDIR'], [13900020, 'EINVAL'], [13900021, 'ENFILE'],
  [13900022, 'EMFILE'], [13900024, 'EFBIG'],
  [13900025, 'ENOSPC'], [13900027, 'EROFS'], [13900030, 'ENAMETOOLONG'],
  [13900032, 'ENOTEMPTY'], [13900033, 'ELOOP'], [13900041, 'EDQUOT'],
]);

// The older POSIX operations report these English errors without a code.
const fileioErrors: Map<string, string> = new Map<string, string>([
  ['Operation not permitted', 'EPERM'], ['No such file or directory', 'ENOENT'],
  ['Interrupted system call', 'EINTR'], ['Try again', 'EAGAIN'],
  ['Permission denied', 'EACCES'], ['Device or resource busy', 'EBUSY'],
  ['File exists', 'EEXIST'], ['Not a directory', 'ENOTDIR'],
  ['Is a directory', 'EISDIR'], ['Invalid argument', 'EINVAL'],
  ['File table overflow', 'ENFILE'], ['Too many open files', 'EMFILE'],
  ['No space left on device', 'ENOSPC'], ['Read-only file system', 'EROFS'],
  ['Filename too long', 'ENAMETOOLONG'], ['File name too long', 'ENAMETOOLONG'],
  ['Directory not empty', 'ENOTEMPTY'], ['Too many symbolic links encountered', 'ELOOP'],
  ['Quota exceeded', 'EDQUOT'],
]);

class FileError extends Error {
  readonly code: string;

  constructor(code: string) {
    super(code);
    this.code = code;
  }
}

interface Paths {
  data: string;
  cache: string;
  temp: string;
}

// The engine page owns these handles. They are independent of the indexes
// the system gives its descriptors and never alias stdin, stdout or stderr.
export class SandboxFiles {
  private files: Map<number, number> = new Map<number, number>();
  private next: number = 3;
  private paths: Paths;
  private roots: string[];
  private base64: util.Base64Helper = new util.Base64Helper();

  constructor(data: string, cache: string, temp: string) {
    this.paths = { data: data, cache: cache, temp: temp };
    this.roots = [data, cache, temp];
  }

  close(): void {
    this.files.forEach((fd: number) => {
      try {
        fs.closeSync(fd);
      } catch (e) {
        console.warn('NSGo sandbox file close: ' + e);
      }
    });
    this.files.clear();
  }

  private path(path: string, allowMissing: boolean = false, useRoot: boolean = true): string {
    const root = this.roots.find((dir: string) => path === dir || path.startsWith(dir + '/'));
    if (root === undefined || path.includes('\u0000') || (!useRoot && path === root)) {
      throw new FileError('EACCES');
    }
    if (path === root) {
      return path;
    }
    const children = path.substring(root.length + 1).split('/');
    let current = root;
    for (let i = 0; i < children.length; i++) {
      const name = children[i];
      if (name === '' || name === '.' || name === '..') {
        throw new FileError('EACCES');
      }
      current += '/' + name;
      try {
        if (fs.lstatSync(current).isSymbolicLink()) {
          throw new FileError('ELOOP');
        }
      } catch (e) {
        const code = this.errorCode(e);
        if (!allowMissing || i !== children.length - 1 || code !== 'ENOENT') {
          throw new FileError(code);
        }
      }
    }
    return current;
  }

  private fd(request: Request): number {
    const fd = this.files.get(request.fd ?? -1);
    if (fd === undefined) {
      throw new FileError('EBADF');
    }
    return fd;
  }

  private errorCode(error: Error): string {
    if (error instanceof FileError) {
      return error.code;
    }
    const native = error as BusinessError;
    return errorCodes.get(native.code) ?? fileioErrors.get(error.message) ?? 'EIO';
  }

  private stat(value: fs.Stat): string {
    return JSON.stringify({
      size: value.size, mode: value.mode, uid: value.uid, gid: value.gid, ino: Number(value.ino),
      atimeMs: value.atime * 1000, mtimeMs: value.mtime * 1000, ctimeMs: value.ctime * 1000,
      dev: 0, rdev: 0, nlink: 0, blksize: 0, blocks: 0, directory: value.isDirectory(),
    });
  }

  call(operation: string, raw: string): string {
    try {
      const request = JSON.parse(raw) as Request;
      return '{"value":' + this.run(operation, request) + '}';
    } catch (e) {
      return JSON.stringify({ error: this.errorCode(e) });
    }
  }

  private run(operation: string, request: Request): string {
    const path = request.path ?? '';
    switch (operation) {
      case 'paths':
        return JSON.stringify(this.paths);
      case 'open': {
        const flags = request.flags ?? 0;
        const name = this.path(path, (flags & fs.OpenMode.CREATE) !== 0);
        // The POSIX API takes a creation mode and preserves the exclusive
        // create flag used by os.CreateTemp.
        const fd = fileio.openSync(name, flags | fs.OpenMode.NOFOLLOW, request.mode ?? 0);
        const handle = this.next++;
        this.files.set(handle, fd);
        return JSON.stringify(handle);
      }
      case 'close': {
        const fd = this.fd(request);
        this.files.delete(request.fd ?? -1);
        fs.closeSync(fd);
        break;
      }
      case 'fstat':
        return this.stat(fs.statSync(this.fd(request)));
      case 'stat':
      case 'lstat':
        return this.stat(fs.lstatSync(this.path(path)));
      case 'readdir':
        return JSON.stringify(fs.listFileSync(this.path(path)));
      case 'mkdir':
        fileio.mkdirSync(this.path(path, true, false), request.mode ?? 0);
        break;
      case 'unlink':
        fs.unlinkSync(this.path(path, false, false));
        break;
      case 'rmdir':
        // fileIo.rmdirSync recursively removes children. The POSIX API
        // leaves a nonempty directory in place, as os.Remove requires.
        fileio.rmdirSync(this.path(path, false, false));
        break;
      case 'rename':
        fs.renameSync(this.path(path, false, false), this.path(request.target ?? '', true, false));
        break;
      case 'truncate':
        fs.truncateSync(this.path(path, false, false), request.size ?? 0);
        break;
      case 'ftruncate':
        fs.truncateSync(this.fd(request), request.size ?? 0);
        break;
      case 'fsync':
        fs.fsyncSync(this.fd(request));
        break;
      case 'read': {
        const fd = this.fd(request);
        const size = request.size ?? 0;
        if (!Number.isInteger(size) || size < 0 || size > 64 * 1024) {
          throw new FileError('EINVAL');
        }
        if (size === 0) {
          return '""';
        }
        const buffer = new ArrayBuffer(size);
        const count = request.position == null ?
          fs.readSync(fd, buffer) : fs.readSync(fd, buffer, { offset: request.position });
        if (count === 0) {
          return '""';
        }
        return JSON.stringify(this.base64.encodeToStringSync(new Uint8Array(buffer, 0, count)));
      }
      case 'write': {
        const fd = this.fd(request);
        const data = request.data ?? '';
        if (data.length > 4 * Math.ceil(64 * 1024 / 3)) {
          throw new FileError('EINVAL');
        }
        const bytes = this.base64.decodeSync(data);
        const buffer = bytes.slice().buffer as ArrayBuffer;
        return JSON.stringify(request.position == null ?
          fs.writeSync(fd, buffer) : fs.writeSync(fd, buffer, { offset: request.position }));
      }
      default:
        throw new FileError('ENOSYS');
    }
    return 'null';
  }
}
