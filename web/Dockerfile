# 前端运行时镜像：nginx 托管 vite 构建产物（dist/ 由宿主机 pnpm build 生成）
# 构建：minikube image build -t tickethub/web:local web/
# 说明：本机网络拉不到 Docker Hub 的 node 镜像，故不在镜像内跑 pnpm build（构建步骤放宿主机）
FROM nginx:alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY dist/ /usr/share/nginx/html

EXPOSE 80