export interface IAXUser {
  name: string;
  type: string;
  context: string;
  auth: string;
  secret: string;
  host: string;
  disallow: string[];
  allow: string[];
  transfer: string;
}
