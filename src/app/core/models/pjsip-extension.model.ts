export interface PJSIPExtension {
  id: string;
  context: string;
  disallow: string[];
  allow: string[];
  caller_id: string;
  password: string;
  max_contacts: string;
  direct_media: string;
  media_encryption: string;
}
