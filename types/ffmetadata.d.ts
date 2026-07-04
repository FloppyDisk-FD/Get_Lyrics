declare module "ffmetadata" {
  export function setFfmpegPath(path: string): void;

  export function read(
    path: string,
    callback: (err: Error | null, data: Record<string, unknown>) => void
  ): void;

  export function write(
    path: string,
    data: Record<string, unknown>,
    callback: (err: Error | null) => void
  ): void;

  export function write(
    path: string,
    data: Record<string, unknown>,
    options: Record<string, unknown>,
    callback: (err: Error | null) => void
  ): void;
}
