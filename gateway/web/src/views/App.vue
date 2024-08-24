<template>
  <div>
    <el-card style="width: 680px; padding: 20px">
      <template #header>
        <div class="card-header">
          <h2>ZTA统一认证中心</h2>
        </div>
      </template>
      <div class="container">
        <el-form ref="formRef" :model="model" label-position="top" :rules="rules">

          <el-form-item label="用户名" prop="username" required>
            <el-input v-model:model-value="model.username" />
          </el-form-item>
          <el-form-item label="密码" prop="password" required>
            <el-input type="password" v-model:model-value="model.password" />
          </el-form-item>

          <el-form-item>
            <el-button type="primary" @click="handleSubmit">登录</el-button>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
      </template>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import type {FormInstance, FormRules} from "element-plus";
import {reactive, ref} from "vue";

interface FormModel {
  username: string;
  password: string;
}

interface ResponseData {
  redirect_uri: string;
}

interface ResponseModel {
    code: number;
    message: string;
    data: ResponseData;
}

const result = ref<ResponseModel>();
const model = reactive<FormModel>({
  username: "",
  password: "",
});

const rules = reactive<FormRules>({
  username: [
    {
      required: true,
      message: "请输入用户名",
      trigger: "blur",
    },
  ],
  password: [
      {
        required: true,
        message: "请输入密码",
        trigger: "blur",
      },
    ],
});
const formRef = ref<FormInstance>();

const handleSubmit = () => {
  if (!formRef.value) {
    return;
  }

  formRef.value.validate(async (valid) => {
    await submit(model.username, model.password);
  });
};

const submit = async (username: string, password: string) => {
  console.log( window.location.search)
  const response = await fetch("http://oidc.zta.beyondnetwork.net:14001/authorize" +  window.location.search, {
    method: "POST",
    headers: {
      "Content-Type": "application/json; charset=utf-8",
    },
    body: JSON.stringify({
      username,
      password
    }),
  });

  if (!response.ok) {
    alert("请求出错: " + response.status);
    return;
  }

  const data: ResponseModel = await response.json();
  if (data.code != 0) {
    alert("登录出错: " + data.message)
  }

  window.location.href = data.data.redirect_uri
};
</script>

<style lang="css"></style>
