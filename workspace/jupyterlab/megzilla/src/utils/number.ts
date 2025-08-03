export function  storageHumanNumberSuffix(value: number) {
  let data = value;
  const units = ['B', 'K', 'M', 'G', 'T', 'P', 'E', 'Z', 'Y'];
  let u = 0;
  do {
    data /= 1024;
    ++u;
  } while (Math.abs(data) >= 1024 && u < units.length - 1);
  return `${data.toFixed(2)} ${units[u]}`
};
